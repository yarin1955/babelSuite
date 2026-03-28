param(
  [string]$Registry = "localhost:5000"
)

$ErrorActionPreference = "Stop"

$context = Join-Path $PSScriptRoot "zot-seed"
if (-not (Test-Path $context)) {
  throw "Seed context not found at $context"
}

$images = @(
  @{
    SuiteId = "payment-suite"
    Title = "Payment Suite"
    Kind = "suite"
    Repository = "core-platform/payment-suite"
    Version = "v2.4.1"
    Tags = @("v2.4.1", "v2.4.0", "v2.3.8", "latest")
  },
  @{
    SuiteId = "fleet-control-room"
    Title = "Fleet Control Room"
    Kind = "suite"
    Repository = "platform/fleet-control-room"
    Version = "v1.8.0"
    Tags = @("v1.8.0", "v1.7.5", "latest")
  },
  @{
    SuiteId = "identity-broker"
    Title = "Identity Broker"
    Kind = "suite"
    Repository = "security/identity-broker"
    Version = "v3.0.2"
    Tags = @("v3.0.2", "v3.0.1", "latest")
  },
  @{
    SuiteId = "storefront-browser-lab"
    Title = "Storefront Browser Lab"
    Kind = "suite"
    Repository = "qa/storefront-browser-lab"
    Version = "v1.3.0"
    Tags = @("v1.3.0", "v1.2.5", "latest")
  },
  @{
    SuiteId = "stdlib-postgres"
    Title = "@babelsuite/postgres"
    Kind = "stdlib"
    Repository = "babelsuite/postgres"
    Version = "1.4.0"
    Tags = @("1.4.0", "1.3.2", "latest")
  },
  @{
    SuiteId = "stdlib-kafka"
    Title = "@babelsuite/kafka"
    Kind = "stdlib"
    Repository = "babelsuite/kafka"
    Version = "1.2.3"
    Tags = @("1.2.3", "1.2.2", "latest")
  },
  @{
    SuiteId = "fleet-control-room-demo"
    Title = "Fleet Control Room Demo"
    Kind = "example"
    Repository = "examples/fleet-control-room-demo"
    Version = "0.2.0"
    Tags = @("0.2.0", "latest")
  },
  @{
    SuiteId = "fleet-operations-scenarios"
    Title = "Fleet Operations Scenarios"
    Kind = "example"
    Repository = "examples/fleet-operations-scenarios"
    Version = "v0.1.0"
    Tags = @("v0.1.0", "latest")
  }
)

foreach ($image in $images) {
  $primary = "$Registry/$($image.Repository):$($image.Version)"
  Write-Host "Building $primary"
  docker build `
    --build-arg "SUITE_ID=$($image.SuiteId)" `
    --build-arg "SUITE_TITLE=$($image.Title)" `
    --build-arg "SUITE_KIND=$($image.Kind)" `
    --build-arg "SUITE_VERSION=$($image.Version)" `
    -t $primary `
    $context

  foreach ($tag in $image.Tags) {
    $ref = "$Registry/$($image.Repository):$tag"
    if ($ref -ne $primary) {
      docker tag $primary $ref
    }

    Write-Host "Pushing $ref"
    docker push $ref
  }
}

$catalog = Invoke-WebRequest -UseBasicParsing "http://127.0.0.1:5000/v2/_catalog"
Write-Host $catalog.Content
