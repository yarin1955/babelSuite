type Product = {
  sku: string
  name: string
  price: number
}

const products: Product[] = [
  { sku: "sku_1001", name: "Starter Keyboard", price: 4900 },
  { sku: "sku_2024", name: "Launch Headset", price: 12900 },
  { sku: "sku_9000", name: "Flash Cable Bundle", price: 1900 },
]

for (const product of products) {
  console.log(`warming cache for ${product.sku} (${product.name}) at ${product.price}`)
}
