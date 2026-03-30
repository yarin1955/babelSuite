package babelsuite.issuer

default allow := false

allow if {
  input.suite == "identity-broker"
  count(input.modules) >= 1
}
