package babelsuite.event_schema

default allow := false

allow if {
  input.suite == "returns-control-plane"
  count(input.modules) >= 1
}
