package babelsuite.session

default allow := false

allow if {
  input.suite == "identity-broker"
  count(input.modules) >= 1
}
