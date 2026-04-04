from locust import HttpUser, between, task


class LegacyUser(HttpUser):
    wait_time = between(1, 3)

    @task(3)
    def list_products(self):
        self.client.get("/products", name="/products")

    @task(1)
    def get_order(self):
        self.client.get("/orders/ord_123", name="/orders/:id")
