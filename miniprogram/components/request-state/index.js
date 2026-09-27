Component({
  properties: {
    error: { type: String, value: "" },
    retrying: { type: Boolean, value: false },
    compact: { type: Boolean, value: false },
  },
  methods: { retry() { if (!this.data.retrying) this.triggerEvent("retry") } },
})
