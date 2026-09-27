# portfind uninstaller for Windows.
#
#   irm https://raw.githubusercontent.com/rpratap2111/portfind/main/uninstall.ps1 | iex
#
# Removes the executables, the PATH entry and the Start menu shortcuts added by
# install.ps1. Your kill history (%LOCALAPPDATA%\portfind\history.db) is kept
# unless you set:
#   $env:PORTFIND_PURGE_HISTORY = "1"
# If you installed to a custom directory, set $env:PORTFIND_INSTALL_DIR to it.

# Remove-PortfindFromPath drops Dir from the user PATH in the registry,
# preserving the other entries unexpanded.
function Remove-PortfindFromPath([string]$Dir, [string]$RegistryKey = 'Environment') {
    $key = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey($RegistryKey, $true)
    if (-not $key) { return $false }
    try {
        $raw = [string]$key.GetValue('Path', '', [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames)
        $entries = @($raw -split ';' | Where-Object { $_ })
        $target = $Dir.TrimEnd('\')
        $kept = @($entries | Where-Object { [Environment]::ExpandEnvironmentVariables($_).TrimEnd('\') -ine $target })
        if ($kept.Count -eq $entries.Count) { return $false }
        $key.SetValue('Path', ($kept -join ';'), [Microsoft.Win32.RegistryValueKind]::ExpandString)
    } finally {
        $key.Close()
    }
    if ($RegistryKey -eq 'Environment') {
        [Environment]::SetEnvironmentVariable('PORTFIND_INSTALLER', '1', 'User') # broadcast the change
        [Environment]::SetEnvironmentVariable('PORTFIND_INSTALLER', $null, 'User')
    }
    return $true
}

# Remove-PortfindShortcuts deletes the Start menu shortcuts install.ps1 made,
# only if they still point at a portfind executable.
function Remove-PortfindShortcuts([string]$ProgramsDir = [Environment]::GetFolderPath('Programs')) {
    $shell = New-Object -ComObject WScript.Shell
    $removed = @()
    foreach ($name in 'portfind', 'portfind (terminal)') {
        $path = Join-Path $ProgramsDir "$name.lnk"
        if (-not (Test-Path $path)) { continue }
        $target = $shell.CreateShortcut($path).TargetPath
        if ((Split-Path $target -Leaf) -notin 'portfind.exe', 'portfind-tray.exe') { continue }
        Remove-Item -Force -Path $path
        $removed += $name
    }
    return $removed
}

function Uninstall-Portfind {
    $ErrorActionPreference = 'Stop'

    $dataDir = Join-Path $env:LOCALAPPDATA 'portfind'
    $installDir = if ($env:PORTFIND_INSTALL_DIR) { $env:PORTFIND_INSTALL_DIR } else { Join-Path $dataDir 'bin' }

    $running = @(Get-Process -Name 'portfind', 'portfind-tray' -ErrorAction SilentlyContinue)
    if ($running.Count -gt 0) {
        throw "portfind is still running (PID $($running.Id -join ', ')). Close it and run the uninstaller again."
    }

    # Delete only portfind's own files: a custom install dir may be shared
    # with other tools.
    $exes = @(Get-ChildItem -Path $installDir -Filter 'portfind*.exe' -File -ErrorAction SilentlyContinue)
    if ($exes.Count -eq 0) {
        Write-Host "No portfind executables found in $installDir"
    }
    foreach ($exe in $exes) {
        Remove-Item -Force -Path $exe.FullName
        Write-Host "Removed $($exe.FullName)"
    }
    if ((Test-Path $installDir) -and -not (Get-ChildItem -Path $installDir -Force | Select-Object -First 1)) {
        Remove-Item -Force -Path $installDir
    }
    if (Remove-PortfindFromPath $installDir) {
        Write-Host "Removed $installDir from your user PATH."
    }
    $removed = @(Remove-PortfindShortcuts)
    if ($removed.Count -gt 0) {
        Write-Host "Removed Start menu shortcuts: $($removed -join ', ')"
    }

    if ($env:PORTFIND_PURGE_HISTORY -eq '1') {
        if (Test-Path $dataDir) {
            Remove-Item -Recurse -Force -Path $dataDir
            Write-Host "Removed history in $dataDir"
        }
    } elseif (Test-Path (Join-Path $dataDir 'history.db')) {
        Write-Host "Kept your history in $dataDir (set `$env:PORTFIND_PURGE_HISTORY = '1' to delete it too)."
    }
    Write-Host "portfind uninstalled." -ForegroundColor Green
}

Uninstall-Portfind
