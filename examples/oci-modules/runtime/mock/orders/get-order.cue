"$schema": "https://schemas.babelsuite.dev/mock-exchange-source-v1.json"
suite: "runtime-module-example"
artifact: "mock/orders/get-order.cue"
operationId: "get-order"
adapter: "rest"
dispatcher: "apisix"
examples: {
  "approved": {
    dispatch: [{from:"path", param:"id", value:"ord_123"}]
    responseSchema: {
      status: "200"
      mediaType: "application/json"
      body: {
        id: "ord_123"
        status: "approved"
        traceId: string @gen(kind="uuid")
      }
    }
  }
}
