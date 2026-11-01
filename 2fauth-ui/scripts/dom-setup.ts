/**
 * Creates the jsdom window and publishes the DOM globals before React loads.
 * Imported first by smoke.ts so that `react-dom` sees a browser environment.
 */
import { JSDOM } from 'jsdom'

export const dom = new JSDOM('<!doctype html><html><head></head><body><div id="root"></div></body></html>', {
  url: 'http://localhost:8123/',
  pretendToBeVisual: true,
})

const { window } = dom
const globals = globalThis as unknown as Record<string, unknown>
globals.window = window
globals.document = window.document
// node defines navigator as a getter; only patch it when absent
if (!('navigator' in globalThis)) globals.navigator = window.navigator
globals.HTMLElement = window.HTMLElement
globals.Element = window.Element
globals.Node = window.Node
globals.CustomEvent = window.CustomEvent
globals.MouseEvent = window.MouseEvent
globals.requestAnimationFrame = window.requestAnimationFrame.bind(window)
globals.cancelAnimationFrame = window.cancelAnimationFrame.bind(window)
globals.getComputedStyle = window.getComputedStyle.bind(window)
globals.localStorage = window.localStorage

export const errors: string[] = []
window.addEventListener('error', (event) => errors.push(String(event.error ?? event.message)))
process.on('uncaughtException', (error) => errors.push(String(error)))
