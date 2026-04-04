package suites

func seedSOAPClaimsHubSuite() Definition {
	return Definition{
		ID:          "soap-claims-hub",
		Title:       "SOAP Claims Hub",
		Repository:  "localhost:5000/enterprise-integration/soap-claims-hub",
		Owner:       "Enterprise Integration",
		Provider:    "Zot",
		Version:     "v0.9.0",
		Tags:        []string{"latest", "v0.9.0", "v0.8.3"},
		Description: "Legacy claims sandbox with a SOAP intake surface, XML envelopes, and APISIX-fronted dispatch into the BabelSuite mock engine.",
		Status:      "Verified",
		Score:       87,
		PullCommand: "babelctl run localhost:5000/enterprise-integration/soap-claims-hub:v0.9.0",
		ForkCommand: "babelctl fork localhost:5000/enterprise-integration/soap-claims-hub:v0.9.0 ./soap-claims-hub-local",
		SuiteStar: `load("@babelsuite/runtime", "container", "mock", "script", "scenario")

claims_mock = mock(name="claims-mock")
seed_reference_data = script(name="seed-reference-data", after=["claims-mock"])
claims_bridge = container.run(name="claims-bridge", after=["claims-mock", "seed-reference-data"])
claims_smoke = scenario(name="claims-smoke", after=["claims-bridge"])`,
		Profiles: []ProfileOption{
			{FileName: "local.yaml", Label: "Local Debug", Description: "Relaxed SOAP timeouts with verbose envelope logging.", Default: true},
			{FileName: "canary.yaml", Label: "Canary Partner", Description: "Partner-like SOAP headers and stricter claim review toggles."},
		},
		Folders: []FolderEntry{
			{Name: "profiles", Role: "Core", Description: "SOAP endpoint runtime knobs and partner header defaults.", Files: []string{"local.yaml", "canary.yaml"}},
			{Name: "api", Role: "Core", Description: "WSDL contract published to legacy claim submitters.", Files: []string{"wsdl/claims.wsdl"}},
			{Name: "mock", Role: "Core", Description: "Schema-driven SOAP mock definitions that render XML envelopes at runtime.", Files: []string{"claims/claim-service.json"}},
			{Name: "scripts", Role: "Core", Description: "Bootstrap hooks for claim code tables and partner fixtures.", Files: []string{"seed_reference_data.sh"}},
			{Name: "scenarios", Role: "Core", Description: "Smoke coverage for submit and lookup SOAP exchanges.", Files: []string{"claims_smoke.py"}},
			{Name: "fixtures", Role: "Core", Description: "Seeded partner claims and policy fixtures.", Files: []string{"claims.json"}},
			{Name: "policies", Role: "Core", Description: "SOAP fault and envelope validation policies.", Files: []string{"soap_faults.rego"}},
		},
		SeedSources: soapClaimsHubSeedSources,
		Contracts: []string{
			"The SOAP endpoint is fronted by APISIX, but BabelSuite still owns the request/response mock generation behind the sidecar.",
			"WSDL stays under api/ while schema-backed mock definitions in mock/ drive runtime XML envelopes.",
			"SOAPAction is treated as the primary dispatch key so legacy XML clients do not need JSON-specific adapters.",
		},
		APISurfaces: []APISurface{
			{
				ID:          "claims-soap",
				Title:       "Claims SOAP Service",
				Protocol:    "SOAP",
				MockHost:    "https://soap-claims-hub.mock.internal",
				Description: "SOAP 1.1 claim intake and status lookup routed through APISIX into the shared BabelSuite mock engine.",
				Operations: []APIOperation{
					{
						ID:           "claim-service",
						Method:       "POST",
						Name:         "/ClaimService",
						Summary:      "Accept SubmitClaim and GetClaimStatus SOAP actions on the shared claims endpoint.",
						ContractPath: "api/wsdl/claims.wsdl#ClaimService",
						MockPath:     "mock/claims/claim-service.json",
						MockURL:      "https://soap-claims-hub.mock.internal/ClaimService",
						CurlCommand: `curl -X POST "https://soap-claims-hub.mock.internal/ClaimService" -H "content-type: text/xml; charset=utf-8" -H "soapaction: urn:SubmitClaim" -H "x-suite-profile: canary.yaml" -d '<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/" xmlns:clm="urn:claims:v1">
  <soapenv:Header/>
  <soapenv:Body>
    <clm:SubmitClaimRequest>
      <clm:PolicyNumber>PL-1024</clm:PolicyNumber>
      <clm:LossType>water</clm:LossType>
      <clm:Amount>4200</clm:Amount>
    </clm:SubmitClaimRequest>
  </soapenv:Body>
</soapenv:Envelope>'`,
						MockMetadata: MockOperationMetadata{
							Adapter:     "rest",
							DelayMillis: 90,
							ParameterConstraints: []ParameterConstraint{
								{Name: "content-type", Source: "header", Required: true, Pattern: "(?i)^(text/xml|application/soap\\+xml)"},
								{Name: "soapaction", Source: "header", Required: true, Pattern: "^urn:(SubmitClaim|GetClaimStatus)$"},
								{Name: "x-suite-profile", Source: "header", Forward: true},
							},
							Fallback: &MockFallback{
								Mode:      "static",
								Status:    "500",
								MediaType: "text/xml; charset=utf-8",
								Body: `<?xml version="1.0" encoding="UTF-8"?>
<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/">
  <soapenv:Body>
    <soapenv:Fault>
      <faultcode>soapenv:Client</faultcode>
      <faultstring>No SOAPAction matched the incoming request.</faultstring>
    </soapenv:Fault>
  </soapenv:Body>
</soapenv:Envelope>`,
							},
						},
						Exchanges: []ExchangeExample{
							{
								Name:           "SubmitClaim",
								SourceArtifact: "mock/claims/claim-service.json",
								When:           []MatchCondition{{From: "header", Param: "soapaction", Value: "urn:SubmitClaim"}},
								RequestHeaders: []Header{
									{Name: "content-type", Value: "text/xml; charset=utf-8"},
									{Name: "soapaction", Value: "urn:SubmitClaim"},
									{Name: "x-suite-profile", Value: "canary.yaml"},
								},
								RequestBody: `<?xml version="1.0" encoding="UTF-8"?>
<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/" xmlns:clm="urn:claims:v1">
  <soapenv:Header/>
  <soapenv:Body>
    <clm:SubmitClaimRequest>
      <clm:PolicyNumber>PL-1024</clm:PolicyNumber>
      <clm:LossType>water</clm:LossType>
      <clm:Amount>4200</clm:Amount>
    </clm:SubmitClaimRequest>
  </soapenv:Body>
</soapenv:Envelope>`,
								ResponseStatus:    "200",
								ResponseMediaType: "text/xml; charset=utf-8",
								ResponseHeaders: []Header{
									{Name: "x-mock-source", Value: "soap-submit-claim"},
								},
								ResponseBody: `<?xml version="1.0" encoding="UTF-8"?>
<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/" xmlns:clm="urn:claims:v1">
  <soapenv:Body>
    <clm:SubmitClaimResponse>
      <clm:ClaimId>clm_2048</clm:ClaimId>
      <clm:Decision>APPROVED</clm:Decision>
      <clm:Profile>canary.yaml</clm:Profile>
      <clm:TraceId>00000000-0000-0000-0000-000000000010</clm:TraceId>
      <clm:ServedAt>2026-01-01T00:00:00Z</clm:ServedAt>
    </clm:SubmitClaimResponse>
  </soapenv:Body>
</soapenv:Envelope>`,
							},
							{
								Name:           "GetClaimStatus",
								SourceArtifact: "mock/claims/claim-service.json",
								When:           []MatchCondition{{From: "header", Param: "soapaction", Value: "urn:GetClaimStatus"}},
								RequestHeaders: []Header{
									{Name: "content-type", Value: "text/xml; charset=utf-8"},
									{Name: "soapaction", Value: "urn:GetClaimStatus"},
								},
								RequestBody: `<?xml version="1.0" encoding="UTF-8"?>
<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/" xmlns:clm="urn:claims:v1">
  <soapenv:Header/>
  <soapenv:Body>
    <clm:GetClaimStatusRequest>
      <clm:ClaimId>clm_2048</clm:ClaimId>
    </clm:GetClaimStatusRequest>
  </soapenv:Body>
</soapenv:Envelope>`,
								ResponseStatus:    "200",
								ResponseMediaType: "text/xml; charset=utf-8",
								ResponseHeaders: []Header{
									{Name: "x-mock-source", Value: "soap-claim-status"},
								},
								ResponseBody: `<?xml version="1.0" encoding="UTF-8"?>
<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/" xmlns:clm="urn:claims:v1">
  <soapenv:Body>
    <clm:GetClaimStatusResponse>
      <clm:ClaimId>clm_2048</clm:ClaimId>
      <clm:Status>IN_REVIEW</clm:Status>
      <clm:Owner>manual_queue</clm:Owner>
      <clm:TraceId>00000000-0000-0000-0000-000000000011</clm:TraceId>
      <clm:ServedAt>2026-01-01T00:05:00Z</clm:ServedAt>
    </clm:GetClaimStatusResponse>
  </soapenv:Body>
</soapenv:Envelope>`,
							},
						},
					},
				},
			},
		},
	}
}
