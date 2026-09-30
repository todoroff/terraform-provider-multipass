[CmdletBinding()]
param(
    [string]$TerraformPath = 'terraform'
)

$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path -Parent $PSScriptRoot
$terraformCommand = (Get-Command $TerraformPath -CommandType Application -ErrorAction Stop).Source
$utf8 = New-Object System.Text.UTF8Encoding($false)

# Check local inline and reference-style Markdown links, including llms.txt
# links to this repository's master branch. External URLs and anchors are not
# fetched; CI should not depend on third-party websites being available.
$files = & git -C $repoRoot ls-files --cached --others --exclude-standard -- '*.md' llms.txt
if ($LASTEXITCODE -ne 0) { throw 'Unable to list documentation files' }
$brokenLinks = @()
$linkCount = 0
$repositoryURL = 'https://raw.githubusercontent.com/todoroff/terraform-provider-multipass/master/'
foreach ($file in ($files | Sort-Object -Unique)) {
    $fullPath = Join-Path $repoRoot $file
    # git ls-files also includes tracked files deleted by a pending rename.
    if (-not (Test-Path -LiteralPath $fullPath -PathType Leaf)) { continue }
    $content = [IO.File]::ReadAllText($fullPath)
    $content = [regex]::Replace($content, '(?ms)^\s*(```|~~~)[^\r\n]*\r?\n.*?^\s*\1\s*$', '')
    $links = @([regex]::Matches($content, '\]\((?<target><[^>]+>|[^\s)]+)(?:\s+"[^"]*")?\)'))
    $links += @([regex]::Matches($content, '(?m)^ {0,3}\[[^\]\r\n]+\]:\s*(?<target><[^>]+>|\S+)'))
    foreach ($link in $links) {
        $target = $link.Groups['target'].Value.Trim('<', '>')
        $basePath = Split-Path -Parent $fullPath
        if ($target.StartsWith($repositoryURL, [StringComparison]::Ordinal)) {
            $target = $target.Substring($repositoryURL.Length)
            $basePath = $repoRoot
        } elseif ($target -match '^([a-zA-Z][a-zA-Z0-9+.-]*:|//|#)') {
            continue
        } elseif ($target.StartsWith('/')) {
            $target = $target.TrimStart('/')
            $basePath = $repoRoot
        }
        $relativePath = [Uri]::UnescapeDataString(($target -split '[#?]', 2)[0])
        $linkCount++
        if (-not (Test-Path -LiteralPath (Join-Path $basePath $relativePath))) {
            $brokenLinks += "${file}: $($link.Groups['target'].Value)"
        }
    }
}
if ($brokenLinks.Count -gt 0) {
    throw ("Broken documentation links:`n" + ($brokenLinks -join "`n"))
}
Write-Host "Validated $linkCount local documentation links."

$buildRoot = [IO.Path]::GetFullPath((Join-Path $repoRoot 'build/docs-validation'))
$workDir = Join-Path $buildRoot ([Guid]::NewGuid().ToString('N'))
$binDir = Join-Path $workDir 'bin'
New-Item -ItemType Directory -Path $binDir -Force | Out-Null
$exeSuffix = if ($env:OS -eq 'Windows_NT') { '.exe' } else { '' }

# Use isolated CLI settings and a development override for the actual local
# build. Schema inspection neither configures Multipass nor creates resources.
$cliEnvironment = @{
    TF_CLI_CONFIG_FILE = Join-Path $workDir 'terraform.rc'
    TF_DATA_DIR = Join-Path $workDir '.terraform'
    TF_IN_AUTOMATION = '1'
    TF_INPUT = '0'
    TF_REATTACH_PROVIDERS = $null
    TF_CLI_ARGS = $null
    TF_CLI_ARGS_providers = $null
    TF_LOG = $null
    TF_LOG_PATH = $null
}
$savedEnvironment = @{}
foreach ($key in $cliEnvironment.Keys) {
    $savedEnvironment[$key] = [Environment]::GetEnvironmentVariable($key, 'Process')
}

Push-Location $repoRoot
try {
    & go build -o (Join-Path $binDir "terraform-provider-multipass$exeSuffix") ./cmd/terraform-provider-multipass
    if ($LASTEXITCODE -ne 0) { throw 'Provider build failed' }

    $config = @'
terraform {
  required_providers {
    multipass = {
      source = "registry.terraform.io/todoroff/multipass"
    }
  }
}
'@
    [IO.File]::WriteAllText((Join-Path $workDir 'main.tf'), $config, $utf8)
    $hclBinDir = ConvertTo-Json -InputObject ($binDir.Replace('\', '/')) -Compress
    $cliConfig = @"
provider_installation {
  dev_overrides {
    "registry.terraform.io/todoroff/multipass" = $hclBinDir
  }
}
"@
    [IO.File]::WriteAllText($cliEnvironment.TF_CLI_CONFIG_FILE, $cliConfig, $utf8)
    foreach ($key in $cliEnvironment.Keys) {
        [Environment]::SetEnvironmentVariable($key, $cliEnvironment[$key], 'Process')
    }

    $schemaJSON = & $terraformCommand "-chdir=$workDir" providers schema -json
    if ($LASTEXITCODE -ne 0) { throw 'Provider schema export failed' }
    $schema = ($schemaJSON -join "`n") | ConvertFrom-Json
    $providerSchema = $schema.provider_schemas.'registry.terraform.io/todoroff/multipass'
    if ($null -eq $providerSchema) { throw 'Local Multipass provider schema is missing' }

    # tfplugindocs 0.25.0 accepts the short name or a hashicorp/ address when
    # reading schema JSON. Normalize only its input, retaining our real source
    # address for the build and CLI handshake above.
    $docsSchema = @{
        format_version = $schema.format_version
        provider_schemas = @{ multipass = $providerSchema }
    } | ConvertTo-Json -Depth 100
    $schemaPath = Join-Path $workDir 'schema.json'
    [IO.File]::WriteAllText($schemaPath, $docsSchema, $utf8)

    & go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs@v0.25.0 validate --provider-name multipass --provider-dir $repoRoot --providers-schema $schemaPath
    if ($LASTEXITCODE -ne 0) { throw 'Provider documentation validation failed' }
} finally {
    foreach ($key in $savedEnvironment.Keys) {
        [Environment]::SetEnvironmentVariable($key, $savedEnvironment[$key], 'Process')
    }
    Pop-Location
    $resolvedWorkDir = [IO.Path]::GetFullPath($workDir)
    $buildPrefix = $buildRoot + [IO.Path]::DirectorySeparatorChar
    if (-not $resolvedWorkDir.StartsWith($buildPrefix, [StringComparison]::OrdinalIgnoreCase)) {
        throw "Refusing to remove a directory outside $buildRoot"
    }
    Remove-Item -LiteralPath $resolvedWorkDir -Recurse -Force
}
