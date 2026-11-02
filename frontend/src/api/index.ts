/**
 * Stable import surface for generated Wails bindings.
 */
export * from '@bindings/tobiro/app'
import { Events, System, Window as WailsWindow } from '@wailsio/runtime'

export { Events, System, WailsWindow }

export function EventsOn<T = unknown>(event: string, handler: (data: T) => void): () => void {
  return Events.On(event, (ev) => {
    handler(ev.data as T)
  })
}

export function Environment() {
  return System.Environment()
}

export function WindowSetBackgroundColour(r: number, g: number, b: number, a: number) {
  return WailsWindow.SetBackgroundColour(r, g, b, a)
}
