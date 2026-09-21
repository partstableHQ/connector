# winget manifest template — Partstable.Connector

Submitted as a PR to microsoft/winget-pkgs after the first **public**
release exists (the manifest pins the released installer URL and its
SHA256 — both unknown until then). Fill the placeholders from the GitHub
release page, validate with `winget validate`, then follow
https://github.com/microsoft/winget-pkgs/blob/master/CONTRIBUTING.md.

Files (this directory, one PR):
- `Partstable.Connector.yaml` (version)
- `Partstable.Connector.installer.yaml`
- `Partstable.Connector.locale.en-US.yaml`
- `Partstable.Connector.license.yaml`

## Version manifest

```yaml
PackageIdentifier: Partstable.Connector
PackageVersion: __VERSION__
DefaultLocale: en-US
ManifestType: version
ManifestVersion: 1.6.0
```

## Installer manifest

```yaml
PackageIdentifier: Partstable.Connector
PackageVersion: __VERSION__
Installers:
  - Architecture: x64
    InstallerType: zip
    NestedInstallerType: portable
    NestedInstallerFiles:
      - RelativeFilePath: partstable.exe
        PortableCommandAlias: partstable
    InstallerUrl: https://github.com/partstableHQ/connector/releases/download/v__VERSION__/partstable-connector___VERSION___windows_amd64.zip
    InstallerSha256: __SHA256__
ManifestType: installer
ManifestVersion: 1.6.0
```

## Locale manifest

```yaml
PackageIdentifier: Partstable.Connector
PackageVersion: __VERSION__
PackageLocale: en-US
Publisher: PartsTable
PackageName: PartsTable Connector
License: MIT
ShortDescription: The parts compendium, on your computer.
Description: |-
  Look up any part. Paste a whole list. Get the description, the
  substitutes, and the source of every fact — instantly, offline, from a
  local database that ships with the app.
Moniker: partstable
Tags:
  - parts
  - hardware
  - itad
  - cross-reference
ManifestType: defaultLocale
ManifestVersion: 1.6.0
```

## License manifest

```yaml
PackageIdentifier: Partstable.Connector
PackageVersion: __VERSION__
License: MIT
LicenseUrl: https://github.com/partstableHQ/connector/blob/main/LICENSE
Copyright: Copyright (c) 2026 PartsTable
ShortDescription: The parts compendium, on your computer.
ManifestType: license
ManifestVersion: 1.6.0
```
