// Command mkconnector generates BESPOKE integration-connector bundles —
// the FM-25 product (CEO scope amendment 2026-10-04: custom-made for each
// company). A bundle is a connector.yaml manifest (what system, how to log
// in, which hosts it may talk to, the field map to the parts schema) plus
// an ed25519 signature over the manifest bytes. The app refuses unsigned or
// tampered bundles outside dev mode.
//
//	foo@bar:~$ mkconnector generate --name "Dabergy Odoo" --system odoo \
//	    --auth oauth2 --host erp.dabergy.com --fieldmap odoo-map.json \
//	    --out bundle/ --key dabergy.key
//	foo@bar:~$ mkconnector verify --bundle bundle/ --pub dabergy.pub
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// minCoreVersion guards which app releases may load the bundle.
const minCoreVersion = "v0.1.0"

type bundleSpec struct {
	Name      string            // human name, e.g. "Dabergy Odoo"
	System    string            // odoo | quickbooks | salesforce | zoho | custom
	Auth      string            // oauth2 | apikey | basic | qbwc
	Hosts     []string          // the ONLY endpoint hosts the connector may talk to
	FieldMap  map[string]string // parts-schema field -> system field
	Schedule  string            // sync schedule, cron-ish descriptor
	CreatedBy string
	CreatedAt string
}

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "generate":
		generate(os.Args[2:])
	case "verify":
		verify(os.Args[2:])
	case "keygen":
		keygen(os.Args[2:])
	default:
		usage()
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `mkconnector — build bespoke, signed integration-connector bundles

  generate  --name NAME --system SYSTEM --auth AUTH [--host HOST]...
            [--fieldmap map.json] [--schedule SCH] [--out DIR] [--key FILE]
            Build connector.yaml + connector.sig (ed25519). Generates the
            keypair next to the bundle when --key is given a new path.
  verify    --bundle DIR [--pub FILE]
            Check the bundle signature against a public key.
  keygen    --out FILE
            Write an ed25519 private key (and FILE.pub).`)
	os.Exit(2)
}

func generate(args []string) {
	fs := flag.NewFlagSet("generate", flag.ExitOnError)
	name := fs.String("name", "", "human name of this bespoke connector (required)")
	system := fs.String("system", "custom", "target system: odoo|quickbooks|salesforce|zoho|custom")
	auth := fs.String("auth", "apikey", "auth type: oauth2|apikey|basic|qbwc")
	schedule := fs.String("schedule", "manual", "sync schedule descriptor")
	out := fs.String("out", "connector-bundle", "output directory")
	keyPath := fs.String("key", "", "ed25519 private key (generated if missing)")
	fieldMapPath := fs.String("fieldmap", "", "JSON file mapping parts-schema fields to system fields")
	createdBy := fs.String("created-by", "", "who built this bundle (company/builder)")
	var hosts arrayFlags
	fs.Var(&hosts, "host", "allowed endpoint host (repeatable, required)")
	_ = fs.Parse(args)

	if *name == "" || len(hosts) == 0 {
		fmt.Fprintln(os.Stderr, "mkconnector: --name and at least one --host are required")
		os.Exit(2)
	}
	if *system != "odoo" && *system != "quickbooks" && *system != "salesforce" && *system != "zoho" && *system != "custom" {
		fmt.Fprintf(os.Stderr, "mkconnector: unknown --system %q\n", *system)
		os.Exit(2)
	}
	if *auth != "oauth2" && *auth != "apikey" && *auth != "basic" && *auth != "qbwc" {
		fmt.Fprintf(os.Stderr, "mkconnector: unknown --auth %q\n", *auth)
		os.Exit(2)
	}

	fieldMap := map[string]string{}
	if *fieldMapPath != "" {
		raw, err := os.ReadFile(*fieldMapPath)
		if err != nil {
			die("read fieldmap: %v", err)
		}
		if err := json.Unmarshal(raw, &fieldMap); err != nil {
			die("fieldmap %s is not valid JSON: %v", *fieldMapPath, err)
		}
	}

	spec := bundleSpec{
		Name: *name, System: *system, Auth: *auth,
		Hosts: hosts, FieldMap: fieldMap, Schedule: *schedule,
		CreatedBy: *createdBy, CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	manifest, err := renderManifest(spec)
	if err != nil {
		die("render manifest: %v", err)
	}

	// Key: use the given one, or generate a fresh pair for this bespoke build.
	var priv ed25519.PrivateKey
	if *keyPath == "" {
		_, priv, err = ed25519.GenerateKey(rand.Reader)
		if err != nil {
			die("generate key: %v", err)
		}
	} else {
		priv = loadOrCreateKey(*keyPath)
	}
	sig := ed25519.Sign(priv, manifest)

	if err := os.MkdirAll(*out, 0o755); err != nil {
		die("mkdir %s: %v", *out, err)
	}
	if err := os.WriteFile(filepath.Join(*out, "connector.yaml"), manifest, 0o644); err != nil {
		die("write manifest: %v", err)
	}
	sigText := fmt.Sprintf("unofficial minisign-compatible: ed25519\n%s\n",
		base64.StdEncoding.EncodeToString(sig))
	if err := os.WriteFile(filepath.Join(*out, "connector.sig"), []byte(sigText), 0o644); err != nil {
		die("write signature: %v", err)
	}
	pubFile := filepath.Join(*out, "connector.pub")
	if err := os.WriteFile(pubFile, []byte(base64.StdEncoding.EncodeToString(priv.Public().(ed25519.PublicKey))+"\n"), 0o644); err != nil {
		die("write public key: %v", err)
	}

	fmt.Printf("bespoke connector bundle written:\n  %s\n  %s\n  %s\n"+
		"hosts: %s\nKEEP the private key with the company record; the app needs the .pub to verify.\n",
		filepath.Join(*out, "connector.yaml"), filepath.Join(*out, "connector.sig"), pubFile,
		strings.Join(hosts, ", "))
}

func renderManifest(s bundleSpec) ([]byte, error) {
	fmJSON, err := json.Marshal(s.FieldMap)
	if err != nil {
		return nil, err
	}
	hosts := make([]string, 0, len(s.Hosts))
	for _, h := range s.Hosts {
		hosts = append(hosts, fmt.Sprintf("  - %q", strings.TrimSpace(h)))
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# Bespoke PartsTable integration connector — generated by mkconnector\n")
	fmt.Fprintf(&b, "# Signed bundle: the app refuses unsigned or tampered manifests (FM-25).\n")
	fmt.Fprintf(&b, "apiVersion: 1\n")
	fmt.Fprintf(&b, "kind: connector\nname: %q\nsystem: %q\nauth: %q\n", s.Name, s.System, s.Auth)
	fmt.Fprintf(&b, "allowedHosts:\n%s\n", strings.Join(hosts, "\n"))
	fmt.Fprintf(&b, "schedule: %q\nminCoreVersion: %q\n", s.Schedule, minCoreVersion)
	fmt.Fprintf(&b, "credentials: keychain # BYO only — the OS keychain, never PartsTable\n")
	fmt.Fprintf(&b, "fieldMap: %s\n", fmJSON)
	fmt.Fprintf(&b, "createdBy: %q\ncreatedAt: %q\n", s.CreatedBy, s.CreatedAt)
	return []byte(b.String()), nil
}

func verify(args []string) {
	if err := runVerify(args); err != nil {
		fmt.Fprintln(os.Stderr, "mkconnector:", err)
		os.Exit(1)
	}
}

func runVerify(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ExitOnError)
	bundle := fs.String("bundle", "connector-bundle", "bundle directory")
	pubPath := fs.String("pub", "", "public key (defaults to connector.pub in the bundle)")
	_ = fs.Parse(args)

	manifest, err := os.ReadFile(filepath.Join(*bundle, "connector.yaml"))
	if err != nil {
		return fmt.Errorf("read manifest: %w", err)
	}
	sigRaw, err := os.ReadFile(filepath.Join(*bundle, "connector.sig"))
	if err != nil {
		return fmt.Errorf("read signature: %w", err)
	}
	pubFile := *pubPath
	if pubFile == "" {
		pubFile = filepath.Join(*bundle, "connector.pub")
	}
	pubRaw, err := os.ReadFile(pubFile)
	if err != nil {
		return fmt.Errorf("read public key: %w", err)
	}
	sigLines := strings.Split(strings.TrimSpace(string(sigRaw)), "\n")
	if len(sigLines) < 2 {
		return fmt.Errorf("signature file malformed")
	}
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(sigLines[len(sigLines)-1]))
	if err != nil {
		return fmt.Errorf("signature is not valid base64: %w", err)
	}
	pub, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(pubRaw)))
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return fmt.Errorf("public key malformed")
	}
	if !ed25519.Verify(ed25519.PublicKey(pub), manifest, sig) {
		return fmt.Errorf("INVALID — the manifest does not match the signature; refuse to load")
	}
	fmt.Println("OK — the bundle is intact and signed by the holder of the key.")
	return nil
}

func keygen(args []string) {
	fs := flag.NewFlagSet("keygen", flag.ExitOnError)
	out := fs.String("out", "connector.key", "private key file")
	_ = fs.Parse(args)
	priv := loadOrCreateKey(*out)
	fmt.Printf("key ready: %s\npublic: %s.pub\n%s",
		*out, *out, base64.StdEncoding.EncodeToString(priv.Public().(ed25519.PublicKey))+"\n")
}

// loadOrCreateKey reads an ed25519 private key (base64 file) or creates and
// stores a fresh one. The key file is the company's signing identity.
func loadOrCreateKey(path string) ed25519.PrivateKey {
	if raw, err := os.ReadFile(path); err == nil {
		key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
		if err != nil || len(key) != ed25519.PrivateKeySize {
			die("%s is not a valid ed25519 private key file", path)
		}
		return ed25519.PrivateKey(key)
	}
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		die("generate key: %v", err)
	}
	if err := os.WriteFile(path, []byte(base64.StdEncoding.EncodeToString(priv)+"\n"), 0o600); err != nil {
		die("write key: %v", err)
	}
	return priv
}

type arrayFlags []string

func (a *arrayFlags) String() string     { return strings.Join(*a, ", ") }
func (a *arrayFlags) Set(v string) error { *a = append(*a, v); return nil }

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "mkconnector: "+format+"\n", args...)
	os.Exit(1)
}
