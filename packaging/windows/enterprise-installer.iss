; PartsTable Connector — COMPANY installer (Inno Setup 7).
; The company-level install of the scope ruling (docs/SCOPE-A2A.md §7):
; run ONCE by management on a machine — per-MACHINE (Program Files, all
; users), the pairing that follows names the COMPANY. This is the security
; key that binds the company to PartsTable over A2A; per-employee installs
; use the regular per-user installer instead.
; Built by: ISCC enterprise-installer.iss

#define MyAppName "PartsTable Connector"
#define MyAppVersion "0.1.0-beta.30"
#define MyAppPublisher "PartsTable"
#define MyAppExeName "partstable.exe"

; Distinct AppId: a company install and a per-user install are different
; animals and may coexist on one machine without uninstalling each other.
[Setup]
AppId={{9C3F1E7A-2B64-4D85-A1C9-E50F83D2B7A4}
AppName={#MyAppName} (Company)
AppVersion={#MyAppVersion}
AppPublisher={#MyAppPublisher}
DefaultDirName={autopf}\{#MyAppName}
DisableProgramGroupPage=yes
OutputDir=dist
OutputBaseFilename=PartstableConnector-{#MyAppVersion}-company-setup
Compression=lzma2/max
SolidCompression=yes
WizardStyle=modern
PrivilegesRequired=admin
UninstallDisplayName={#MyAppName} (Company)
UninstallDisplayIcon={app}\{#MyAppExeName}

[Files]
Source: "partstable.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "compendium-beta.bin"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
Name: "{commonprograms}\{#MyAppName}"; Filename: "{app}\{#MyAppName}"
Name: "{commondesktop}\{#MyAppName}"; Filename: "{app}\{#MyAppName}"

[Run]
Filename: "{app}\{#MyAppExeName}"; \
    Parameters: "install-compendium ""{app}\compendium-beta.bin"""; \
    Flags: runhidden
Filename: "{app}\{#MyAppExeName}"; \
    Description: "{cm:LaunchProgram,{#MyAppName}}"; \
    Flags: nowait postinstall skipifsilent

[Code]
procedure CurStepChanged(CurStep: TSetupStep);
begin
  if CurStep = ssPostInstall then
    MsgBox('Company install complete.' #13#10 +
           'Next: sign in ONCE on this machine as the COMPANY account — every ' +
           'A2A message this Connector sends then carries the company identity. ' +
           'Employee accounts are not required; the license is per company.',
           mbInformation, MB_OK);
end;
