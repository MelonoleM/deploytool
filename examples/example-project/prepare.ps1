# Builds every locally sourced demo artifact. No package manager or network access is used.
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $MyInvocation.MyCommand.Path
$build = Join-Path $root 'build'
New-Item -ItemType Directory -Force -Path $build | Out-Null

# jar.exe is usually absent from PATH even when java/javac are available through the
# Oracle javapath shims, so resolve it from JAVA_HOME, PATH, javac's own directory, or
# the common JDK install roots before giving up with a clear message.
function Find-JarTool {
  if ($env:JAVA_HOME) {
    $candidate = Join-Path $env:JAVA_HOME 'bin/jar.exe'
    if (Test-Path $candidate) { return $candidate }
  }
  $onPath = Get-Command jar -ErrorAction SilentlyContinue
  if ($onPath) { return $onPath.Source }
  $javac = Get-Command javac -ErrorAction SilentlyContinue
  if ($javac) {
    $candidate = Join-Path (Split-Path -Parent $javac.Source) 'jar.exe'
    if (Test-Path $candidate) { return $candidate }
  }
  $roots = @(
    (Join-Path $env:ProgramFiles 'Java'),
    (Join-Path $env:ProgramFiles 'Eclipse Adoptium'),
    (Join-Path $env:ProgramFiles 'Microsoft'),
    (Join-Path $env:ProgramFiles 'Zulu'),
    (Join-Path $env:ProgramFiles 'Amazon Corretto')
  )
  foreach ($r in $roots) {
    if (-not (Test-Path $r)) { continue }
    $found = Get-ChildItem -Path $r -Filter jar.exe -Recurse -ErrorAction SilentlyContinue | Select-Object -First 1
    if ($found) { return $found.FullName }
  }
  return $null
}

go build -trimpath -o (Join-Path $build 'bootstrap-exe.exe') (Join-Path $root 'services/go')
go build -trimpath -o (Join-Path $build 'nginx-proxy.exe') (Join-Path $root 'services/go')
javac -d $build (Join-Path $root 'services/java/ExampleServer.java') (Join-Path $root 'services/java/JarServer.java')
Copy-Item (Join-Path $root 'services/python/app.py') (Join-Path $build 'python-app.py') -Force
Copy-Item (Join-Path $root 'services/node/app.js') (Join-Path $build 'node-app.js') -Force

$jarTool = Find-JarTool
if (-not $jarTool) {
  throw 'jar.exe was not found. Install a JDK or set JAVA_HOME so the jar demo service can be packaged.'
}
$jarPath = Join-Path $build 'java-jar-api.jar'
Remove-Item -LiteralPath $jarPath -Force -ErrorAction SilentlyContinue
& $jarTool --create --file $jarPath --main-class JarServer -C $build JarServer.class
if ($LASTEXITCODE -ne 0) { throw "jar packaging failed with exit code $LASTEXITCODE" }

Write-Host "Prepared offline demo artifacts in $build"
