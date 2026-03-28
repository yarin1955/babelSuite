package storefront.latency

default allow = false

allow if {
  input.checkout_ms <= 2500
}
