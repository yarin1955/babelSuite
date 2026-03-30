package babelsuite.pci

default allow := false

allow if {
  input.suite == "payment-suite"
  count(input.modules) >= 1
}
