package babelsuite.checkout_latency

default allow := false

allow if {
  input.suite == "storefront-browser-lab"
  count(input.modules) >= 1
}
