package babelsuite.refund_limits

default allow := false

allow if {
  input.suite == "returns-control-plane"
  count(input.modules) >= 1
}
