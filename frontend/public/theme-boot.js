/*
 * Pre-paint theme boot (plan D19). Loaded as a blocking same-origin script in
 * <head>, before any stylesheet, so the stored theme is on <html> before the
 * first paint — no flash. It never validates themes: useTheme stores only an
 * already-validated record. It re-checks each value against a cheap grammar
 * before writing it and falls back to the default on any error.
 *
 * Mirrors src/theme/apply.ts and the storage key/version in
 * src/composables/ui/useTheme.ts; themeBoot.contract.component.test.ts proves the
 * two produce identical <html> state. Plain ES5, no imports: it runs before the
 * app bundle.
 */
;(function () {
  'use strict'

  var KEY = 'admin-ui:theme-applied'
  var VERSION = 1
  var DEFAULT_PRESET = 'retro-dark-soft'
  var LIGHT_PRESET = 'retro-paper'
  var SCHEMES = { 'retro-dark': 'dark', 'retro-dark-soft': 'dark', 'retro-dark-lifted': 'dark', 'retro-paper': 'light' }
  var TOKEN_NAME = /^[a-z][a-z0-9-]{0,39}$/
  var TOKEN_VALUE = /^(?:\d{1,3} \d{1,3} \d{1,3}|\d{1,4}(?:\.\d{1,2})?(?:px|ms))$/
  var root = document.documentElement

  function systemPreset() {
    try {
      return window.matchMedia && window.matchMedia('(prefers-color-scheme: light)').matches ? LIGHT_PRESET : DEFAULT_PRESET
    } catch (e) {
      return DEFAULT_PRESET
    }
  }

  function apply(presetId, overrides) {
    for (var name in overrides) {
      if (Object.prototype.hasOwnProperty.call(overrides, name)) {
        var value = overrides[name]
        if (TOKEN_NAME.test(name) && typeof value === 'string' && TOKEN_VALUE.test(value)) {
          root.style.setProperty('--' + name, value)
        }
      }
    }
    root.setAttribute('data-theme', presetId)
    root.classList.toggle('dark', SCHEMES[presetId] === 'dark')
  }

  function stored() {
    var raw = window.localStorage.getItem(KEY)
    if (!raw) return null
    var envelope = JSON.parse(raw)
    var theme = envelope && envelope.version === VERSION ? envelope.data : null
    if (!theme || !Object.prototype.hasOwnProperty.call(SCHEMES, theme.presetId)) return null
    return theme
  }

  try {
    var theme = stored()
    if (theme) apply(theme.presetId, theme.overrides && typeof theme.overrides === 'object' ? theme.overrides : {})
    else apply(systemPreset(), {})
  } catch (e) {
    apply(DEFAULT_PRESET, {})
  }
})()
