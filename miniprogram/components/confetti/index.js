// 彩纸庆祝：对应 Web 端 lib/confetti（记录成功、盲盒揭晓时触发）。
// 页面放 <confetti id="confetti" />，调用 this.selectComponent("#confetti").fire()
const COLORS = ["#E8734A", "#6EC6B8", "#F5D76E", "#F4A8A0", "#8B5CF6", "#FFB38A"]

Component({
  data: { pieces: [] },

  lifetimes: {
    detached() { clearTimeout(this._timer) },
  },

  methods: {
    fire() {
      const seed = Date.now()
      const pieces = []
      for (let i = 0; i < 42; i++) {
        const round = Math.random() > 0.72
        const width = Math.round(10 + Math.random() * 10)
        pieces.push({
          id: `${seed}-${i}`,
          left: Math.round(Math.random() * 100),
          color: COLORS[i % COLORS.length],
          width,
          height: round ? width : Math.round(16 + Math.random() * 14),
          round,
          variant: i % 4,
          duration: (1.7 + Math.random() * 1.3).toFixed(2),
          delay: (Math.random() * 0.4).toFixed(2),
        })
      }
      this.setData({ pieces })
      clearTimeout(this._timer)
      this._timer = setTimeout(() => this.setData({ pieces: [] }), 3600)
    },
  },
})
