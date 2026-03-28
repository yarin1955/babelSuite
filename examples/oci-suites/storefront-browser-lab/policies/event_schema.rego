package storefront.events

default allow = false

allow if {
  input.orderId != ""
  input.sku != ""
  input.email != ""
  input.status == "accepted"
}
