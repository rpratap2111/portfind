# portfind installer for Windows. No admin rights needed.
#
#   irm https://raw.githubusercontent.com/rpratap2111/portfind/main/install.ps1 | iex
#
# It downloads the release zip for your CPU, verifies its SHA-256 against the
# release's checksums.txt, copies the executables to the install directory and
# adds that directory to your user PATH.
#
# Options (set before running the command above):
#   $env:PORTFIND_VERSION         = "v0.1.0"            # default: latest release
#   $env:PORTFIND_INSTALL_DIR     = "D:\tools\portfind" # default: %LOCALAPPDATA%\portfind\bin
#   $env:PORTFIND_NO_MODIFY_PATH  = "1"                 # don't touch PATH
#   $env:PORTFIND_DOWNLOAD_BASE   = "https://..."       # mirror hosting the release files

# Everything runs inside functions so that, under `irm | iex`, settings such as
# $ErrorActionPreference don't leak into the caller's session.

function Get-PortfindArch {
    $arch = $null
    try {
        $arch = [string][System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture
    } catch {
        $arch = if ($env:PROCESSOR_ARCHITEW6432) { $env:PROCESSOR_ARCHITEW6432 } else { $env:PROCESSOR_ARCHITECTURE }
    }
    switch -Regex ($arch) {
        '^(X64|AMD64)$' { return 'amd64' }
        '^(Arm64|ARM64)$' { return 'arm64' }
        default { throw "portfind needs 64-bit Windows (x64 or ARM64); this machine reports '$arch'." }
    }
}

function Get-PortfindFile([string]$Url, [string]$OutFile) {
    try {
        Invoke-WebRequest -Uri $Url -OutFile $OutFile -UseBasicParsing
    } catch {
        $status = $null
        if ($_.Exception.Response) { $status = [int]$_.Exception.Response.StatusCode }
        if ($status -eq 404) {
            throw "Not found: $Url`nCheck that a release exists at https://github.com/rpratap2111/portfind/releases (and that PORTFIND_VERSION, if set, is a real tag)."
        }
        throw "Download failed: $Url`n$($_.Exception.Message)"
    }
}

function Test-PortfindChecksum([string]$File, [string]$ChecksumsFile) {
    $name = Split-Path $File -Leaf
    $line = Get-Content $ChecksumsFile | Where-Object { ($_ -split '\s+')[-1].TrimStart('*') -eq $name } | Select-Object -First 1
    if (-not $line) { throw "checksums.txt has no entry for $name; refusing to install an unverified download." }
    $expected = ($line -split '\s+')[0].ToLowerInvariant()
    $actual = (Get-FileHash -Algorithm SHA256 -Path $File).Hash.ToLowerInvariant()
    if ($expected -ne $actual) {
        throw "Checksum mismatch for $name (expected $expected, got $actual). Nothing was installed."
    }
}

# Add-PortfindToPath appends Dir to the user PATH in the registry. The value is
# read and written unexpanded so entries like %USERPROFILE%\bin survive.
function Add-PortfindToPath([string]$Dir, [string]$RegistryKey = 'Environment') {
    $key = [Microsoft.Win32.Registry]::CurrentUser.CreateSubKey($RegistryKey)
    try {
        $raw = [string]$key.GetValue('Path', '', [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames)
        $entries = @($raw -split ';' | Where-Object { $_ })
        $target = $Dir.TrimEnd('\')
        foreach ($e in $entries) {
            if ([Environment]::ExpandEnvironmentVariables($e).TrimEnd('\') -ieq $target) { return $false }
        }
        $key.SetValue('Path', (($entries + $Dir) -join ';'), [Microsoft.Win32.RegistryValueKind]::ExpandString)
    } finally {
        $key.Close()
    }
    if ($RegistryKey -eq 'Environment') {
        # Setting any user variable through .NET broadcasts WM_SETTINGCHANGE,
        # so terminals opened from now on see the new PATH.
        [Environment]::SetEnvironmentVariable('PORTFIND_INSTALLER', '1', 'User')
        [Environment]::SetEnvironmentVariable('PORTFIND_INSTALLER', $null, 'User')
    }
    return $true
}

function Install-Portfind {
    $ErrorActionPreference = 'Stop'
    $ProgressPreference = 'SilentlyContinue' # the progress bar slows downloads badly in Windows PowerShell 5.1

    if ($PSVersionTable.PSVersion.Major -lt 6) {
        # Windows PowerShell 5.1 may default to TLS 1.0, which GitHub rejects.
        [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
    }

    $repo = 'rpratap2111/portfind'
    $installDir = if ($env:PORTFIND_INSTALL_DIR) { $env:PORTFIND_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA 'portfind\bin' }
    $base = if ($env:PORTFIND_DOWNLOAD_BASE) { $env:PORTFIND_DOWNLOAD_BASE.TrimEnd('/') }
            elseif ($env:PORTFIND_VERSION) { "https://github.com/$repo/releases/download/$($env:PORTFIND_VERSION)" }
            else { "https://github.com/$repo/releases/latest/download" }

    $asset = "portfind_windows_$(Get-PortfindArch).zip"
    $tmp = Join-Path ([IO.Path]::GetTempPath()) ("portfind-install-" + [guid]::NewGuid().ToString('N'))
    New-Item -ItemType Directory -Path $tmp | Out-Null
    try {
        Write-Host "Downloading $asset ..."
        $zip = Join-Path $tmp $asset
        $sums = Join-Path $tmp 'checksums.txt'
        Get-PortfindFile "$base/$asset" $zip
        Get-PortfindFile "$base/checksums.txt" $sums
        Test-PortfindChecksum $zip $sums
        Write-Host "Checksum verified."

        $extracted = Join-Path $tmp 'extracted'
        Expand-Archive -Path $zip -DestinationPath $extracted
        $exes = @(Get-ChildItem -Path $extracted -Filter '*.exe' -Recurse)
        if ($exes.Count -eq 0) { throw "$asset contains no executables." }

        New-Item -ItemType Directory -Force -Path $installDir | Out-Null
        foreach ($exe in $exes) {
            $dest = Join-Path $installDir $exe.Name
            try {
                Copy-Item -Path $exe.FullName -Destination $dest -Force
            } catch {
                throw "Could not write $dest. If portfind (or its tray app) is running, close it and run the installer again.`n$($_.Exception.Message)"
            }
            Unblock-File -Path $dest
        }
    } finally {
        Remove-Item -Recurse -Force -Path $tmp -ErrorAction SilentlyContinue
    }

    if ($env:PORTFIND_NO_MODIFY_PATH -ne '1') {
        if (Add-PortfindToPath $installDir) {
            Write-Host "Added $installDir to your user PATH."
        }
    }
    # Make `portfind` work in this window right away, too.
    if (-not (($env:Path -split ';') | Where-Object { $_.TrimEnd('\') -ieq $installDir.TrimEnd('\') })) {
        $env:Path = "$env:Path;$installDir"
    }

    $installed = & (Join-Path $installDir 'portfind.exe') --version
    Write-Host ""
    Write-Host "Installed $installed to $installDir" -ForegroundColor Green
    Write-Host "Run it with:  portfind"
    Write-Host "(In other terminal windows that were already open, restart them first.)"
}

Install-Portfind
