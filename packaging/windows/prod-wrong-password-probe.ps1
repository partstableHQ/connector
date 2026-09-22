# Live probe: jaime@dabergy.com must reject a wrong password (HTTP 401)
# and never issue a redirect.
$ErrorActionPreference = 'Stop'

$verifierBytes = [Text.Encoding]::UTF8.GetBytes('probe-verifier')
$sha = [Security.Cryptography.SHA256]::Create()
$challenge = [Convert]::ToBase64String($sha.ComputeHash($verifierBytes)).TrimEnd('=')

$params = [ordered]@{
    response_type         = 'code'
    client_id             = 'connector-desktop'
    redirect_uri          = 'http://127.0.0.1:61234/callback'
    state                 = 'probe'
    code_challenge        = $challenge
    code_challenge_method = 'S256'
    scope                 = 'api'
}
$qs = ($params.GetEnumerator() | ForEach-Object {
    "$($_.Key)=$([uri]::EscapeDataString($_.Value))"
}) -join '&'

$form = (Invoke-WebRequest -UseBasicParsing "https://partstable.com/oauth/authorize?$qs").Content

$hidden = @{}
[regex]::Matches($form, 'name="(state|challenge|redirect_uri|client_id)" value="([^"]*)"') | ForEach-Object {
    $hidden[$_.Groups[1].Value] = $_.Groups[2].Value
}
if ($hidden.Count -lt 4) { throw "form missing hidden fields (got $($hidden.Count))" }

$body = @{}
$hidden.GetEnumerator() | ForEach-Object { $body[$_.Key] = $_.Value }
$body['email']    = 'jaime@dabergy.com'
$body['password'] = 'WrongPass999'

$tmp = Join-Path $env:TEMP 'pt-probe.html'
$code = curl.exe -s -o $tmp -w '%{http_code}' -X POST -d (
    ($body.GetEnumerator() | ForEach-Object {
        "$($_.Key)=$([uri]::EscapeDataString($_.Value))"
    }) -join '&'
) 'https://partstable.com/oauth/authorize'

Write-Host "wrong-password status=$code"
$html = Get-Content $tmp -Raw
if ($code -eq '401' -and $html -match 'wrong password') {
    Write-Host 'WRONG-PASSWORD-REJECTED (account protected)'
} else {
    Write-Host "UNEXPECTED: body=$($html.Substring(0, [Math]::Min(300, $html.Length)))"
}
