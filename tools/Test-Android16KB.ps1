param(
    [Parameter(Mandatory = $true)]
    [string]$ApkPath,
    [Parameter(Mandatory = $true)]
    [string]$ZipAlignPath
)

$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.IO.Compression.FileSystem

$apk = (Resolve-Path -LiteralPath $ApkPath).Path
$zipAlign = (Resolve-Path -LiteralPath $ZipAlignPath).Path
$archive = [System.IO.Compression.ZipFile]::OpenRead($apk)
$failures = [System.Collections.Generic.List[string]]::new()
$checked = 0

try {
    foreach ($entry in $archive.Entries) {
        # Android's 16 KB devices use 64-bit ABIs; 32-bit ELF alignment is not required.
        if ($entry.FullName -notmatch '^lib/(arm64-v8a|x86_64)/.+\.so$') {
            continue
        }

        $stream = [System.IO.MemoryStream]::new()
        $source = $entry.Open()
        try {
            $source.CopyTo($stream)
        } finally {
            $source.Dispose()
        }
        $reader = [System.IO.BinaryReader]::new($stream)
        try {
            $stream.Position = 0
            if ($stream.Length -lt 64 -or $reader.ReadUInt32() -ne 0x464C457F) {
                throw "Invalid ELF file: $($entry.FullName)"
            }
            if ($reader.ReadByte() -ne 2 -or $reader.ReadByte() -ne 1) {
                throw "Expected little-endian ELF64: $($entry.FullName)"
            }

            $stream.Position = 32
            $headerOffset = $reader.ReadUInt64()
            $stream.Position = 54
            $headerSize = $reader.ReadUInt16()
            $headerCount = $reader.ReadUInt16()
            if ($headerSize -lt 56 -or $headerCount -eq 0 -or
                $headerOffset + $headerSize * $headerCount -gt $stream.Length) {
                throw "Invalid ELF program headers: $($entry.FullName)"
            }

            $loadCount = 0
            $minimumAlignment = [UInt64]::MaxValue
            for ($index = 0; $index -lt $headerCount; $index++) {
                $position = $headerOffset + $index * $headerSize
                $stream.Position = $position
                if ($reader.ReadUInt32() -ne 1) {
                    continue
                }
                $loadCount++
                $stream.Position = $position + 8
                $offset = $reader.ReadUInt64()
                $address = $reader.ReadUInt64()
                $stream.Position = $position + 48
                $alignment = $reader.ReadUInt64()
                $minimumAlignment = [Math]::Min($minimumAlignment, $alignment)

                if ($alignment -lt 16384 -or
                    ($offset % 16384) -ne ($address % 16384)) {
                    $failures.Add("$($entry.FullName): LOAD[$index] alignment=$alignment offset=$offset address=$address")
                }
            }
            if ($loadCount -eq 0) {
                throw "No LOAD segments: $($entry.FullName)"
            }
            $checked++
            Write-Output "$($entry.FullName): minimum LOAD alignment=$minimumAlignment"
        } finally {
            $reader.Dispose()
        }
    }
} finally {
    $archive.Dispose()
}

if ($checked -eq 0) {
    throw 'No 64-bit native libraries found; compatibility was not verified.'
}

& $zipAlign -c -P 16 4 $apk
if ($LASTEXITCODE -ne 0) {
    $failures.Add('APK ZIP alignment failed (zipalign -c -P 16 4).')
}
if ($failures.Count -gt 0) {
    throw "16 KB compatibility failed:`n$($failures -join "`n")"
}
Write-Output "PASS: $checked native libraries and APK ZIP entries are 16 KB aligned."
