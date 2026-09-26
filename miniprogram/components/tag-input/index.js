// 标签输入：对应 Web 端 TagInput —— 已选标签 + 输入框（逗号 / 回车分隔）+ 常用建议。
// <tag-input value="{{list}}" suggestions="{{common}}" tone="red" bind:change="onChange" />
Component({
  properties: {
    value: { type: Array, value: [] },
    suggestions: { type: Array, value: [] },
    placeholder: { type: String, value: "输入后回车添加" },
    tone: { type: String, value: "primary" },
    max: { type: Number, value: 30 },
  },

  data: { draft: "", focus: false, rest: [] },

  observers: {
    "value, suggestions": function (value, suggestions) {
      const chosen = value || []
      this.setData({ rest: (suggestions || []).filter((item) => chosen.indexOf(item) < 0) })
    },
  },

  methods: {
    emit(list) {
      this.triggerEvent("change", { value: list })
    },

    add(words) {
      const current = (this.data.value || []).slice()
      words.forEach((word) => {
        const text = String(word || "").trim()
        if (text && current.indexOf(text) < 0 && current.length < this.data.max) current.push(text)
      })
      this.emit(current)
    },

    onInput(event) {
      const value = event.detail.value || ""
      // 输入分隔符时立即拆成标签
      if (/[,，、;；\s]$/.test(value)) {
        this.add(value.split(/[,，、;；\s]+/))
        this.setData({ draft: "" })
        return ""
      }
      this.setData({ draft: value })
      return value
    },

    onConfirm() {
      if (!this.data.draft.trim()) return
      this.add(this.data.draft.split(/[,，、;；\s]+/))
      this.setData({ draft: "" })
    },

    onFocus() { this.setData({ focus: true }) },

    onBlur() {
      this.setData({ focus: false })
      if (this.data.draft.trim()) this.onConfirm()
    },

    remove(event) {
      const text = event.currentTarget.dataset.text
      this.emit((this.data.value || []).filter((item) => item !== text))
    },

    pick(event) {
      this.add([event.currentTarget.dataset.text])
    },
  },
})
