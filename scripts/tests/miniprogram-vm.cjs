// Existing business tests exercise the real appearance Page wrapper too.
const vm = require('node:vm')
const fs = require('node:fs')
const path = require('node:path')
const mini = path.resolve(__dirname, '../../miniprogram')
module.exports = {
  ...vm,
  runInNewContext(source, globals = {}, options) {
    const originalRequire = globals.require
    const context = { getCurrentPages: () => [], ...globals }
    let appearance
    context.require = name => {
      if (!name.endsWith('/theme')) return originalRequire?.(name)
      if (!appearance) {
        const module = { exports: {} }
        vm.runInNewContext(fs.readFileSync(path.join(mini, 'utils/theme.js'), 'utf8'), {
          module, wx: context.wx || {}, Page: context.Page, getCurrentPages: context.getCurrentPages,
          require: () => require(path.join(mini, 'utils/theme-palettes.js')),
        })
        appearance = module.exports
      }
      return appearance
    }
    return vm.runInNewContext(source, context, options)
  },
}
