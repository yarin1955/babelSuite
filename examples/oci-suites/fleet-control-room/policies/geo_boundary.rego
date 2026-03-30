package babelsuite.geo_boundary

default allow := false

allow if {
  input.suite == "fleet-control-room"
  count(input.modules) >= 1
}
