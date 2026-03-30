package babelsuite.event_schema

default allow := false

allow if {
  input.suite == "storefront-browser-lab"
  count(input.modules) >= 1
}
