; PartsTable Connector — Windows beta installer (Inno Setup 7).
; Per-user install (no admin), Start-menu + optional desktop icon,
; sample compendium installed on first launch, uninstaller registered
; in Windows Apps & Features with an optional data cleanup.
; Built by: ISCC beta-installer.iss  (staging: files sit next to this script)

#define MyAppName "PartsTable Connector"
#define MyAppVersion "0.1.0-beta.3"
#define MyAppPublisher "PartsTable"
#define MyAppExeName "partstable.exe"

[Setup]
AppId={{B7E4C2A9-5D18-4F63-9A2E-8C4D1F0B6E92}
AppName={#MyAppName}
AppVersion={#MyAppVersion}
AppPublisher={#MyAppPublisher}
DefaultDirName={localappdata}\Programs\{#MyAppName}
DisableProgramGroupPage=yes
OutputDir=dist
OutputBaseFilename=PartstableConnector-{#MyAppVersion}-setup
Compression=lzma2/max
SolidCompression=yes
WizardStyle=modern
PrivilegesRequired=lowest
UninstallDisplayName={#MyAppName}
UninstallDisplayIcon={app}\{#MyAppExeName}

[Tasks]
Name: "desktopicon"; Description: "{cm:CreateDesktopIcon}"; \
    GroupDescription: "{cm:AdditionalIcons}"; Flags: unchecked

[Files]
Source: "partstable.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "compendium-beta.bin"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
Name: "{autoprograms}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"
Name: "{autodesktop}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"; Tasks: desktopicon

[Run]
; Sample data so the beta is usable the moment it launches (quiet).
Filename: "{app}\{#MyAppExeName}"; \
    Parameters: "install-compendium ""{app}\compendium-beta.bin"""; \
    Flags: runhidden
Filename: "{app}\{#MyAppExeName}"; \
    Description: "{cm:LaunchProgram,{#MyAppName}}"; \
    Flags: nowait postinstall skipifsilent

[UninstallDelete]
; The installed files live in {app}; user data in
; {localappdata}\partstable-connector is kept unless the user opts in
; during uninstall (see below).

[Code]
procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
begin
  if CurUninstallStep = usPostUninstall then
  begin
    if MsgBox('Also remove your local database and settings?',
              mbConfirmation, MB_YESNO) = IDYES then
      DelTree(ExpandConstant('{localappdata}\partstable-connector'), True, True, True);
  end;
end;
