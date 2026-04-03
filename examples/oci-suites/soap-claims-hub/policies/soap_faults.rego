package babelsuite.soap_faults

default allow := false

allow if {
  input.suite == "soap-claims-hub"
  count(input.modules) >= 1
}
