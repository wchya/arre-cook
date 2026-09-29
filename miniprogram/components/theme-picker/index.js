const theme = require("../../utils/theme")
const colors = require("../../utils/theme-palettes")
const PALETTES = [
  { id: "lime", name: "青柠" }, { id: "peach", name: "暖桃" },
  { id: "ocean", name: "海盐蓝" }, { id: "berry", name: "莓果" },
]
Component({
  properties: { row: { type: Boolean, value: false } },
  data: {
    open: false, selectedPalette: "lime", selectedMode: "system", persisted: true, appearanceStyle: "",
    palettes: PALETTES.map((p) => ({ ...p, page: colors[p.id].page, card: colors[p.id].card, action: colors[p.id].action, soft: colors[p.id].soft })),
    modes: [{ id: "light", name: "浅色" }, { id: "dark", name: "深色" }, { id: "system", name: "跟随系统" }],
    label: "青柠 · 跟随系统",
  },
  lifetimes: {
    attached() {
      const paint = (p) => this.setData({
        selectedPalette: p.palette, selectedMode: p.mode, persisted: p.persisted, appearanceStyle: p.style,
        label: PALETTES.find((item) => item.id === p.palette).name + " · " + this.data.modes.find((item) => item.id === p.mode).name,
      })
      paint(theme.palette())
      this._offAppearance = theme.subscribe(paint)
    },
    detached() { if (this._offAppearance) this._offAppearance() },
  },
  methods: {
    open() { this.setData({ open: true }) },
    close() { this.setData({ open: false }) },
    choosePalette(event) { theme.set({ palette: event.currentTarget.dataset.value }) },
    chooseMode(event) { theme.set({ mode: event.currentTarget.dataset.value }) },
  },
})
