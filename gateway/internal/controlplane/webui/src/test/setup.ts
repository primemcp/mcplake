import "@testing-library/jest-dom/vitest";

// jsdom has no layout engine, so it doesn't implement ResizeObserver, and
// never reports any element's size. @xyflow/react (SchemaGraph) needs a
// real callback firing with a non-zero size before it'll even draw edges
// between nodes, not just mount without crashing -- so this fakes one
// measurement per observed node instead of a true no-op stub.
class ResizeObserverStub {
  #callback: ResizeObserverCallback;
  constructor(callback: ResizeObserverCallback) {
    this.#callback = callback;
  }
  observe(target: Element) {
    queueMicrotask(() => {
      const entry = { target, contentRect: { width: 150, height: 46 } } as ResizeObserverEntry;
      this.#callback([entry], this as unknown as ResizeObserver);
    });
  }
  unobserve() {}
  disconnect() {}
}
globalThis.ResizeObserver ??= ResizeObserverStub as unknown as typeof ResizeObserver;

// jsdom also has no CSS transform matrix support, which @xyflow/react uses
// to compute node internals once it thinks a node has been measured.
class DOMMatrixReadOnlyStub {
  m22 = 1;
  constructor(transform?: string) {
    const scale = transform?.match(/scale\(([\d.]+)\)/);
    if (scale) this.m22 = parseFloat(scale[1]);
  }
}
globalThis.DOMMatrixReadOnly ??= DOMMatrixReadOnlyStub as unknown as typeof DOMMatrixReadOnly;

// @xyflow/react measures nodes via plain offsetWidth/offsetHeight (not the
// ResizeObserver's contentRect), which jsdom always reports as 0 since it
// has no layout engine -- so a node never leaves its "unmeasured, hidden"
// state, and no edges get drawn between them, without this.
Object.defineProperty(HTMLElement.prototype, "offsetWidth", { configurable: true, value: 150 });
Object.defineProperty(HTMLElement.prototype, "offsetHeight", { configurable: true, value: 46 });
