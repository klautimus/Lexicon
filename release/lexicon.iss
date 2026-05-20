; Lexicon Installer Script for Inno Setup 6+
; Build with: iscc lexicon.iss

#define MyAppName "Lexicon"
#define MyAppVersion "0.1.0"
#define MyAppPublisher "Lexicon"
#define MyAppURL "https://github.com/kevin/lexicon"
#define MyAppExeName "lexicon.exe"

[Setup]
AppId={{A1B2C3D4-E5F6-7890-1234-567890ABCDEF}
AppName={#MyAppName}
AppVersion={#MyAppVersion}
AppPublisher={#MyAppPublisher}
AppPublisherURL={#MyAppURL}
AppSupportURL={#MyAppURL}
AppUpdatesURL={#MyAppURL}
DefaultDirName={autopf}\Lexicon
DisableDirPage=no
DisableProgramGroupPage=no
ChangesAssociations=no
DefaultGroupName={#MyAppName}
OutputDir=.
OutputBaseFilename=LexiconSetup
Compression=lzma2
SolidCompression=yes
WizardStyle=modern
PrivilegesRequired=admin
SetupLogging=yes
SetupIconFile=lexicon.ico

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

[Tasks]
Name: "desktopicon"; Description: "{cm:CreateDesktopIcon}"; GroupDescription: "{cm:AdditionalIcons}"; Flags: unchecked

[Files]
Source: "lexicon.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "tools\spotiflac.exe"; DestDir: "{app}\tools"; Flags: ignoreversion skipifsourcedoesntexist
Source: "tools\yt-dlp.exe"; DestDir: "{app}\tools"; Flags: ignoreversion skipifsourcedoesntexist
Source: "tools\spotdl.exe"; DestDir: "{app}\tools"; Flags: ignoreversion skipifsourcedoesntexist
Source: "tools\ffmpeg.exe"; DestDir: "{app}\tools"; Flags: ignoreversion skipifsourcedoesntexist
Source: "tools\ffprobe.exe"; DestDir: "{app}\tools"; Flags: ignoreversion skipifsourcedoesntexist
Source: "tools\ngrok.exe"; DestDir: "{app}\tools"; Flags: ignoreversion skipifsourcedoesntexist
Source: "tools\poddl.exe"; DestDir: "{app}\tools"; Flags: ignoreversion skipifsourcedoesntexist

[Dirs]
Name: "{app}\data"; Permissions: everyone-modify
Name: "{app}\tools"; Permissions: everyone-modify
Name: "{app}\podcasts"; Permissions: everyone-modify

[Icons]
Name: "{group}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"; WorkingDir: "{app}"
Name: "{group}\{cm:UninstallProgram,{#MyAppName}}"; Filename: "{uninstallexe}"
Name: "{autodesktop}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"; Tasks: desktopicon; WorkingDir: "{app}"

[Run]
Filename: "powershell.exe"; Parameters: "-WindowStyle Hidden -Command ""Start-Process '{app}\{#MyAppExeName}' -WorkingDirectory '{app}'; $port = {code:GetFrontendPort}; $url = 'http://localhost:' + $port; $maxRetries = 30; $retries = 0; while ($retries -lt $maxRetries) {{ try {{ $r = Invoke-WebRequest -Uri $url -UseBasicParsing -TimeoutSec 2 -ErrorAction Stop; if ($r.StatusCode -eq 200) {{ break }} }} catch {{ }}; Start-Sleep -Seconds 1; $retries++ }}; Start-Process $url"""; Description: "{cm:LaunchProgram,{#StringChange(MyAppName, '&', '&&')}}"; Flags: nowait postinstall skipifsilent

[Code]
var
  ApiKeyPage: TWizardPage;
  ApiKeyEdit: TEdit;
  MediaRootsPage: TWizardPage;
  MediaRootsEdit: TEdit;
  OptionalToolsPage: TWizardPage;
  SpotifyClientIDEdit: TEdit;
  SpotifyClientSecretEdit: TEdit;
  PortPage: TWizardPage;
  BackendPortEdit: TEdit;
  FrontendPortEdit: TEdit;
  FrontendPort: String;

// Helper: validate DeepSeek API key format
function IsValidApiKey(const key: String): Boolean;
begin
  Result := (Length(key) > 20) and (Copy(key, 1, 3) = 'sk-');
end;

// DeepSeek API Key page
procedure CreateApiKeyPage;
var
  lbl: TNewStaticText;
begin
  ApiKeyPage := CreateCustomPage(wpSelectDir, 'DeepSeek API Key', 'Configure AI-powered recommendations (optional).');

  lbl := TNewStaticText.Create(ApiKeyPage);
  lbl.Parent := ApiKeyPage.Surface;
  lbl.Top := 0;
  lbl.Caption := 'API Key (leave blank to skip AI features):';
  lbl.Width := ApiKeyPage.SurfaceWidth;

  ApiKeyEdit := TEdit.Create(ApiKeyPage);
  ApiKeyEdit.Parent := ApiKeyPage.Surface;
  ApiKeyEdit.Left := ScaleX(0);
  ApiKeyEdit.Top := ScaleY(20);
  ApiKeyEdit.Width := ApiKeyPage.SurfaceWidth;
  ApiKeyEdit.Height := ScaleY(21);
  ApiKeyEdit.PasswordChar := '*';
end;

procedure BrowseMediaFolder(Sender: TObject);
var
  folder: String;
  current: String;
begin
  if BrowseForFolder('', folder, True) then
  begin
    current := MediaRootsEdit.Text;
    if current <> '' then
      current := current + ';';
    MediaRootsEdit.Text := current + folder;
  end;
end;

// Media Roots page
procedure CreateMediaRootsPage;
var
  browseBtn: TNewButton;
begin
  MediaRootsPage := CreateCustomPage(ApiKeyPage.ID, 'Media Folders', 'Select the folders containing your music library. You can choose multiple folders (semicolon-separated).');

  MediaRootsEdit := TEdit.Create(MediaRootsPage);
  MediaRootsEdit.Parent := MediaRootsPage.Surface;
  MediaRootsEdit.Left := ScaleX(0);
  MediaRootsEdit.Top := ScaleY(0);
  MediaRootsEdit.Width := MediaRootsPage.SurfaceWidth - ScaleX(90);
  MediaRootsEdit.Height := ScaleY(21);

  browseBtn := TNewButton.Create(MediaRootsPage);
  browseBtn.Parent := MediaRootsPage.Surface;
  browseBtn.Left := MediaRootsEdit.Width + ScaleX(8);
  browseBtn.Top := ScaleY(0);
  browseBtn.Width := ScaleX(82);
  browseBtn.Height := ScaleY(21);
  browseBtn.Caption := 'Browse...';
  browseBtn.OnClick := @BrowseMediaFolder;
end;

// Optional Tools page (Spotify credentials only — download tools are bundled)
procedure CreateOptionalToolsPage;
var
  topOffset: Integer;
  lbl: TNewStaticText;
begin
  OptionalToolsPage := CreateCustomPage(MediaRootsPage.ID, 'Spotify Integration', 'Connect your Spotify account (optional). Download tools are bundled automatically.');
  topOffset := 0;

  // Spotify Client ID
  lbl := TNewStaticText.Create(OptionalToolsPage);
  lbl.Parent := OptionalToolsPage.Surface;
  lbl.Top := topOffset;
  lbl.Caption := 'Spotify Client ID (optional):';
  lbl.Width := OptionalToolsPage.SurfaceWidth;
  topOffset := topOffset + ScaleY(18);

  SpotifyClientIDEdit := TEdit.Create(OptionalToolsPage);
  SpotifyClientIDEdit.Parent := OptionalToolsPage.Surface;
  SpotifyClientIDEdit.Top := topOffset;
  SpotifyClientIDEdit.Width := OptionalToolsPage.SurfaceWidth;
  SpotifyClientIDEdit.Height := ScaleY(21);
  topOffset := topOffset + ScaleY(36);

  // Spotify Client Secret
  lbl := TNewStaticText.Create(OptionalToolsPage);
  lbl.Parent := OptionalToolsPage.Surface;
  lbl.Top := topOffset;
  lbl.Caption := 'Spotify Client Secret (optional):';
  lbl.Width := OptionalToolsPage.SurfaceWidth;
  topOffset := topOffset + ScaleY(18);

  SpotifyClientSecretEdit := TEdit.Create(OptionalToolsPage);
  SpotifyClientSecretEdit.Parent := OptionalToolsPage.Surface;
  SpotifyClientSecretEdit.Top := topOffset;
  SpotifyClientSecretEdit.Width := OptionalToolsPage.SurfaceWidth;
  SpotifyClientSecretEdit.Height := ScaleY(21);
  SpotifyClientSecretEdit.PasswordChar := '*';
end;

// Port Configuration page
procedure CreatePortPage;
var
  lbl: TNewStaticText;
begin
  PortPage := CreateCustomPage(OptionalToolsPage.ID, 'Port Configuration', 'Choose server ports (defaults work for most users).');

  // Backend/API Port
  lbl := TNewStaticText.Create(PortPage);
  lbl.Parent := PortPage.Surface;
  lbl.Top := 0;
  lbl.Caption := 'Backend / API Port:';
  lbl.Width := PortPage.SurfaceWidth;

  BackendPortEdit := TEdit.Create(PortPage);
  BackendPortEdit.Parent := PortPage.Surface;
  BackendPortEdit.Top := ScaleY(18);
  BackendPortEdit.Width := ScaleX(80);
  BackendPortEdit.Height := ScaleY(21);
  BackendPortEdit.Text := '8787';

  // Frontend Port
  lbl := TNewStaticText.Create(PortPage);
  lbl.Parent := PortPage.Surface;
  lbl.Top := ScaleY(50);
  lbl.Caption := 'Frontend Port:';
  lbl.Width := PortPage.SurfaceWidth;

  FrontendPortEdit := TEdit.Create(PortPage);
  FrontendPortEdit.Parent := PortPage.Surface;
  FrontendPortEdit.Top := ScaleY(68);
  FrontendPortEdit.Width := ScaleX(80);
  FrontendPortEdit.Height := ScaleY(21);
  FrontendPortEdit.Text := '8787';
end;

procedure InitializeWizard;
begin
  CreateApiKeyPage;
  CreateMediaRootsPage;
  CreateOptionalToolsPage;
  CreatePortPage;
end;

function NextButtonClick(CurPageID: Integer): Boolean;
begin
  Result := True;
  if CurPageID = ApiKeyPage.ID then
  begin
    if (ApiKeyEdit.Text <> '') and not IsValidApiKey(ApiKeyEdit.Text) then
    begin
      MsgBox('The API key does not look valid (should start with "sk-"). Leave blank to skip AI features.', mbError, MB_OK);
      Result := False;
    end;
  end
  else if (CurPageID = MediaRootsPage.ID) and (MediaRootsEdit.Text = '') then
  begin
    MsgBox('Please select at least one media folder.', mbError, MB_OK);
    Result := False;
  end;

  if CurPageID = PortPage.ID then
  begin
    if (BackendPortEdit.Text = '') or (StrToIntDef(BackendPortEdit.Text, 0) <= 0) or (StrToIntDef(BackendPortEdit.Text, 0) > 65535) then
    begin
      MsgBox('Please enter a valid backend port (1-65535).', mbError, MB_OK);
      Result := False;
    end;
    if (FrontendPortEdit.Text = '') or (StrToIntDef(FrontendPortEdit.Text, 0) <= 0) or (StrToIntDef(FrontendPortEdit.Text, 0) > 65535) then
    begin
      MsgBox('Please enter a valid frontend port (1-65535).', mbError, MB_OK);
      Result := False;
    end;
  end;
end;

// Helper: convert backslashes to forward slashes for .env paths
function FwdSlash(const s: String): String;
var
  tmp: String;
begin
  tmp := s;
  StringChangeEx(tmp, '\\', '/', True);
  Result := tmp;
end;
// Write .env file after installation
procedure CurStepChanged(CurStep: TSetupStep);
var
  envPath: String;
  envContent: String;
begin
  if CurStep = ssPostInstall then
  begin
    envPath := FwdSlash(ExpandConstant('{app}\.env'));
    envContent :=
      'PORT=' + BackendPortEdit.Text + #13#10 +
      'DB_PATH=./data/lexicon.db' + #13#10 +
      'MEDIA_ROOTS=' + MediaRootsEdit.Text + #13#10 +
      'DEEPSEEK_API_KEY=' + ApiKeyEdit.Text + #13#10 +
      'DEEPSEEK_MODEL=deepseek-v4-flash' + #13#10 +
      'DEEPSEEK_THINKING=medium' + #13#10 +
      'DEEPSEEK_BASE_URL=https://api.deepseek.com' + #13#10 +
      'SPOTIFY_FRONTEND_URL=http://127.0.0.1:' + FrontendPortEdit.Text + #13#10;

    if SpotifyClientIDEdit.Text <> '' then
      envContent := envContent + 'SPOTIFY_CLIENT_ID=' + SpotifyClientIDEdit.Text + #13#10;
    if SpotifyClientSecretEdit.Text <> '' then
      envContent := envContent + 'SPOTIFY_CLIENT_SECRET=' + SpotifyClientSecretEdit.Text + #13#10;
    envContent := envContent + 'SPOTIFY_REDIRECT_URI=http://127.0.0.1:' + BackendPortEdit.Text + '/api/spotify/callback' + #13#10;

    // Bundled tool paths (always write — tools are bundled with the installer)
    if FileExists(ExpandConstant('{app}\tools\spotiflac.exe')) then
      envContent := envContent + 'SPOTIFLAC_BIN=' + FwdSlash(ExpandConstant('{app}\tools\spotiflac.exe')) + #13#10;
    if FileExists(ExpandConstant('{app}\tools\spotdl.exe')) then
      envContent := envContent + 'SPOTDL_BIN=' + FwdSlash(ExpandConstant('{app}\tools\spotdl.exe')) + #13#10;
    if FileExists(ExpandConstant('{app}\tools\yt-dlp.exe')) then
      envContent := envContent + 'YTDLP_BIN=' + FwdSlash(ExpandConstant('{app}\tools\yt-dlp.exe')) + #13#10;
    if FileExists(ExpandConstant('{app}\tools\ffmpeg.exe')) then
      envContent := envContent + 'FFMPEG_BIN=' + FwdSlash(ExpandConstant('{app}\tools\ffmpeg.exe')) + #13#10;
    if FileExists(ExpandConstant('{app}\tools\ffprobe.exe')) then
      envContent := envContent + 'FFPROBE_BIN=' + FwdSlash(ExpandConstant('{app}\tools\ffprobe.exe')) + #13#10;
    if FileExists(ExpandConstant('{app}\tools\ngrok.exe')) then
      envContent := envContent + 'NGROK_BIN=' + FwdSlash(ExpandConstant('{app}\tools\ngrok.exe')) + #13#10;
    if FileExists(ExpandConstant('{app}\tools\poddl.exe')) then
      envContent := envContent + 'PODDL_BIN=' + FwdSlash(ExpandConstant('{app}\tools\poddl.exe')) + #13#10;
    envContent := envContent + 'PODCAST_DIR=' + FwdSlash(ExpandConstant('{app}\podcasts')) + #13#10;

    SaveStringToFile(envPath, envContent, False);
    FrontendPort := FrontendPortEdit.Text;
  end;
end;

// Called by [Run] to know which port to open in the browser
function GetFrontendPort(Param: String): String;
begin
  Result := FrontendPort;
end;

// Prompt to remove user data during uninstall
procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
var
  dataPath: String;
  podcastsPath: String;
  envPath: String;
  msg: String;
begin
  if CurUninstallStep = usUninstall then
  begin
    msg := 'Do you also want to remove your Lexicon data?' + #13#10 + #13#10 +
           'This will delete:' + #13#10 +
           '  - Database (data\lexicon.db)' + #13#10 +
           '  - Podcasts folder' + #13#10 +
           '  - Configuration (.env)' + #13#10 + #13#10 +
           'Choose Yes to remove all data, No to keep it.';
    if MsgBox(msg, mbConfirmation, MB_YESNO) = IDYES then
    begin
      dataPath := ExpandConstant('{app}\data');
      podcastsPath := ExpandConstant('{app}\podcasts');
      envPath := ExpandConstant('{app}\.env');

      if DirExists(dataPath) then
        DelTree(dataPath, True, True, True);
      if DirExists(podcastsPath) then
        DelTree(podcastsPath, True, True, True);
      if FileExists(envPath) then
        DeleteFile(envPath);
    end;
  end;
end;
