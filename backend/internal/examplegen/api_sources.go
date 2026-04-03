package examplegen

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/babelsuite/babelsuite/internal/suites"
)

func renderGatewaySource(suite suites.Definition, path string) string {
	if content, ok := suites.GeneratedSourceContent(suite, path); ok {
		return content
	}
	return fmt.Sprintf("# %s\n# Gateway preview is not available for %s yet.\n", suite.Title, path)
}

func renderOpenAPISource(suite suites.Definition) string {
	builder := &strings.Builder{}
	builder.WriteString("openapi: 3.1.0\n")
	builder.WriteString("info:\n")
	builder.WriteString(fmt.Sprintf("  title: %s\n", suite.Title))
	builder.WriteString(fmt.Sprintf("  version: %s\n", suite.Version))
	builder.WriteString("servers:\n")
	builder.WriteString(fmt.Sprintf("  - url: https://%s.mock.internal\n", suite.ID))
	builder.WriteString("paths:\n")

	wrotePath := false
	for _, surface := range suite.APISurfaces {
		for _, operation := range surface.Operations {
			method := strings.ToLower(strings.TrimSpace(operation.Method))
			if method == "rpc" || !strings.HasPrefix(operation.Name, "/") {
				continue
			}
			builder.WriteString(fmt.Sprintf("  %s:\n", operation.Name))
			builder.WriteString(fmt.Sprintf("    %s:\n", method))
			builder.WriteString(fmt.Sprintf("      operationId: %s\n", sanitizeIdentifier(operation.ID)))
			builder.WriteString(fmt.Sprintf("      summary: %s\n", operation.Summary))
			builder.WriteString("      responses:\n")
			builder.WriteString(`        "200":` + "\n")
			builder.WriteString("          description: Successful mock response\n")
			wrotePath = true
		}
	}

	if !wrotePath {
		builder.WriteString("  /healthz:\n")
		builder.WriteString("    get:\n")
		builder.WriteString("      operationId: healthz\n")
		builder.WriteString("      summary: Health probe for the suite API.\n")
		builder.WriteString("      responses:\n")
		builder.WriteString(`        "200":` + "\n")
		builder.WriteString("          description: Healthy\n")
	}

	return builder.String()
}

func renderProtoSource(suite suites.Definition, fileName string) string {
	serviceName := sanitizeIdentifier(strings.TrimSuffix(fileName, filepath.Ext(fileName)))
	if serviceName == "" {
		serviceName = sanitizeIdentifier(suite.ID)
	}

	rpcLines := make([]string, 0)
	for _, surface := range suite.APISurfaces {
		for _, operation := range surface.Operations {
			if !strings.EqualFold(operation.Method, "rpc") {
				continue
			}
			name := operation.Name
			if slash := strings.LastIndex(name, "/"); slash >= 0 {
				name = name[slash+1:]
			}
			name = sanitizeIdentifier(name)
			rpcLines = append(rpcLines, fmt.Sprintf("  rpc %s (%sRequest) returns (%sResponse);", name, name, name))
		}
	}
	if len(rpcLines) == 0 {
		rpcLines = append(rpcLines, "  rpc Ping (PingRequest) returns (PingResponse);")
	}

	return strings.Join([]string{
		`syntax = "proto3";`,
		"",
		fmt.Sprintf("package %s.v1;", strings.ReplaceAll(sanitizeIdentifier(suite.ID), "-", "")),
		"",
		fmt.Sprintf("service %sService {", serviceName),
		strings.Join(rpcLines, "\n"),
		"}",
		"",
		"message PingRequest {}",
		"",
		"message PingResponse {",
		"  string status = 1;",
		"}",
	}, "\n") + "\n"
}

func renderWSDLSource(suite suites.Definition, fileName string) string {
	serviceName := sanitizeIdentifier(strings.TrimSuffix(fileName, filepath.Ext(fileName)))
	if serviceName == "" {
		serviceName = sanitizeIdentifier(suite.ID)
	}

	type soapAction struct {
		name string
	}

	actions := make([]soapAction, 0)
	seen := make(map[string]struct{})
	location := "https://" + suite.ID + ".mock.internal"
	for _, surface := range suite.APISurfaces {
		if !strings.EqualFold(surface.Protocol, "SOAP") {
			continue
		}
		for _, operation := range surface.Operations {
			if host := strings.TrimRight(strings.TrimSpace(surface.MockHost), "/"); host != "" && strings.HasPrefix(strings.TrimSpace(operation.Name), "/") {
				location = host + strings.TrimSpace(operation.Name)
			}
			if len(operation.Exchanges) == 0 {
				name := sanitizeIdentifier(operation.ID)
				if name == "" {
					name = "Invoke"
				}
				if _, ok := seen[name]; ok {
					continue
				}
				seen[name] = struct{}{}
				actions = append(actions, soapAction{name: name})
				continue
			}
			for _, exchange := range operation.Exchanges {
				name := sanitizeIdentifier(exchange.Name)
				if name == "" {
					name = sanitizeIdentifier(operation.ID)
				}
				if name == "" {
					name = "Invoke"
				}
				if _, ok := seen[name]; ok {
					continue
				}
				seen[name] = struct{}{}
				actions = append(actions, soapAction{name: name})
			}
		}
	}
	if len(actions) == 0 {
		actions = append(actions, soapAction{name: "Invoke"})
	}

	targetNamespace := "urn:babelsuite:" + strings.ReplaceAll(strings.ToLower(suite.ID), "_", "-")
	portTypeName := serviceName + "PortType"
	bindingName := serviceName + "Binding"
	serviceBlockName := serviceName + "Service"

	lines := []string{
		`<?xml version="1.0" encoding="UTF-8"?>`,
		`<definitions xmlns="http://schemas.xmlsoap.org/wsdl/" xmlns:soap="http://schemas.xmlsoap.org/wsdl/soap/" xmlns:tns="` + targetNamespace + `" xmlns:xsd="http://www.w3.org/2001/XMLSchema" name="` + serviceBlockName + `" targetNamespace="` + targetNamespace + `">`,
		`  <types>`,
		`    <xsd:schema targetNamespace="` + targetNamespace + `">`,
	}
	for _, action := range actions {
		lines = append(lines,
			`      <xsd:element name="`+action.name+`Request" type="xsd:string"/>`,
			`      <xsd:element name="`+action.name+`Response" type="xsd:string"/>`,
		)
	}
	lines = append(lines,
		`    </xsd:schema>`,
		`  </types>`,
		"",
	)
	for _, action := range actions {
		lines = append(lines,
			`  <message name="`+action.name+`Input">`,
			`    <part name="parameters" element="tns:`+action.name+`Request"/>`,
			`  </message>`,
			`  <message name="`+action.name+`Output">`,
			`    <part name="parameters" element="tns:`+action.name+`Response"/>`,
			`  </message>`,
			"",
		)
	}
	lines = append(lines, `  <portType name="`+portTypeName+`">`)
	for _, action := range actions {
		lines = append(lines,
			`    <operation name="`+action.name+`">`,
			`      <input message="tns:`+action.name+`Input"/>`,
			`      <output message="tns:`+action.name+`Output"/>`,
			`    </operation>`,
		)
	}
	lines = append(lines,
		`  </portType>`,
		`  <binding name="`+bindingName+`" type="tns:`+portTypeName+`">`,
		`    <soap:binding transport="http://schemas.xmlsoap.org/soap/http" style="document"/>`,
	)
	for _, action := range actions {
		lines = append(lines,
			`    <operation name="`+action.name+`">`,
			`      <soap:operation soapAction="urn:`+action.name+`"/>`,
			`      <input><soap:body use="literal"/></input>`,
			`      <output><soap:body use="literal"/></output>`,
			`    </operation>`,
		)
	}
	lines = append(lines,
		`  </binding>`,
		`  <service name="`+serviceBlockName+`">`,
		`    <port name="`+serviceName+`Port" binding="tns:`+bindingName+`">`,
		`      <soap:address location="`+location+`"/>`,
		`    </port>`,
		`  </service>`,
		`</definitions>`,
	)

	return strings.Join(lines, "\n") + "\n"
}
