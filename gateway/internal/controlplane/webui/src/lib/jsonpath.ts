/**
 * A small RFC 9535 JSONPath *selector* -- just enough to evaluate the
 * `ClaimRule.path` expressions the gateway's policy engine accepts
 * (`router/claimrule.go`, via github.com/theory/jsonpath) against a
 * decoded JWT payload, so the Request path tab can simulate claim
 * matching in the browser (#81; the design doc's Non-goals rule out a
 * backend preview endpoint).
 *
 * Supported: `$`, member names (`.name`, `['name']`, `["name"]`),
 * wildcards (`.*`, `[*]`), indexes (`[0]`, `[-1]`), slices (`[a:b:c]`),
 * unions (`['a','b']`, `[0,2]`) and descendant segments (`..name`,
 * `..[*]`). For these the result -- the list of selected nodes in
 * document order -- matches the Go implementation, including "a missing
 * member selects nothing, it is not an error".
 *
 * Not supported: filter selectors (`[?...]`) and function extensions.
 * Those throw `JSONPathError` rather than silently returning nothing, so
 * the UI can say "can't simulate this rule" instead of showing a wrong
 * pass/fail. A policy using them is still valid on the backend.
 */

export class JSONPathError extends Error {
  /** True when the path is valid RFC 9535 that this subset simply does not
   * implement (filter selectors), as opposed to malformed. The gateway
   * evaluates the former normally, so the UI must not present it as a
   * gateway failure -- see requestPath.ts. */
  readonly unsupported: boolean;

  constructor(message: string, unsupported = false) {
    super(message);
    this.name = "JSONPathError";
    this.unsupported = unsupported;
  }
}

type Selector =
  | { kind: "name"; name: string }
  | { kind: "wildcard" }
  | { kind: "index"; index: number }
  | { kind: "slice"; start: number | null; end: number | null; step: number | null };

type Segment = { descendant: boolean; selectors: Selector[] };

// RFC 9535's member-name-shorthand is alpha/underscore/non-ASCII; `-` is
// accepted here too, leniently, since a saved rule already passed the Go
// parser and nothing is gained by being stricter than it in a preview.
const NAME_CHAR = /[A-Za-z0-9_\-\u0080-\uFFFF]/;

class Parser {
  #src: string;
  #pos = 0;

  constructor(src: string) {
    this.#src = src;
  }

  parse(): Segment[] {
    if (this.#src[0] !== "$") throw new JSONPathError("path must start with $");
    this.#pos = 1;
    const segments: Segment[] = [];
    while (this.#pos < this.#src.length) {
      segments.push(this.#segment());
    }
    return segments;
  }

  #peek(offset = 0): string {
    return this.#src[this.#pos + offset] ?? "";
  }

  #fail(what: string): never {
    throw new JSONPathError(`${what} at position ${this.#pos} in ${JSON.stringify(this.#src)}`);
  }

  #unsupported(what: string): never {
    throw new JSONPathError(`${what} in ${JSON.stringify(this.#src)}`, true);
  }

  #skipSpace() {
    while (/\s/.test(this.#peek())) this.#pos++;
  }

  #segment(): Segment {
    const c = this.#peek();
    if (c === ".") {
      if (this.#peek(1) === ".") {
        // Descendant segment: `..name`, `..*` or `..[...]`.
        this.#pos += 2;
        if (this.#peek() === "[") return { descendant: true, selectors: this.#bracket() };
        return { descendant: true, selectors: [this.#shorthand()] };
      }
      this.#pos++;
      return { descendant: false, selectors: [this.#shorthand()] };
    }
    if (c === "[") return { descendant: false, selectors: this.#bracket() };
    this.#fail(`unexpected ${JSON.stringify(c)}`);
  }

  /** `.name` or `.*` -- the part after the dot(s). */
  #shorthand(): Selector {
    if (this.#peek() === "*") {
      this.#pos++;
      return { kind: "wildcard" };
    }
    const start = this.#pos;
    while (NAME_CHAR.test(this.#peek())) this.#pos++;
    if (this.#pos === start) this.#fail("expected a member name or *");
    return { kind: "name", name: this.#src.slice(start, this.#pos) };
  }

  /** `[ selector (, selector)* ]` */
  #bracket(): Selector[] {
    this.#pos++; // [
    const selectors: Selector[] = [];
    for (;;) {
      this.#skipSpace();
      selectors.push(this.#bracketSelector());
      this.#skipSpace();
      const c = this.#peek();
      if (c === ",") {
        this.#pos++;
        continue;
      }
      if (c === "]") {
        this.#pos++;
        return selectors;
      }
      this.#fail(c === "" ? "unterminated [" : `unexpected ${JSON.stringify(c)} inside []`);
    }
  }

  #bracketSelector(): Selector {
    const c = this.#peek();
    if (c === "*") {
      this.#pos++;
      return { kind: "wildcard" };
    }
    if (c === "'" || c === '"') return { kind: "name", name: this.#quoted(c) };
    if (c === "?") this.#unsupported("filter selectors are not supported by this simulator");
    if (c === "" || c === "]") this.#fail("empty selector");
    return this.#indexOrSlice();
  }

  #quoted(quote: string): string {
    this.#pos++; // opening quote
    let out = "";
    for (;;) {
      const c = this.#peek();
      if (c === "") this.#fail("unterminated quoted name");
      this.#pos++;
      if (c === quote) return out;
      if (c === "\\") {
        const esc = this.#peek();
        this.#pos++;
        switch (esc) {
          case "n":
            out += "\n";
            break;
          case "t":
            out += "\t";
            break;
          case "r":
            out += "\r";
            break;
          case "b":
            out += "\b";
            break;
          case "f":
            out += "\f";
            break;
          case "/":
          case "\\":
          case "'":
          case '"':
            out += esc;
            break;
          case "u": {
            const hex = this.#src.slice(this.#pos, this.#pos + 4);
            if (!/^[0-9a-fA-F]{4}$/.test(hex)) this.#fail("bad \\u escape");
            out += String.fromCharCode(parseInt(hex, 16));
            this.#pos += 4;
            break;
          }
          default:
            this.#fail(`bad escape \\${esc}`);
        }
        continue;
      }
      out += c;
    }
  }

  #int(): number | null {
    const start = this.#pos;
    if (this.#peek() === "-") this.#pos++;
    while (/[0-9]/.test(this.#peek())) this.#pos++;
    const text = this.#src.slice(start, this.#pos);
    if (text === "" || text === "-") {
      this.#pos = start;
      return null;
    }
    return parseInt(text, 10);
  }

  #indexOrSlice(): Selector {
    const first = this.#int();
    this.#skipSpace();
    if (this.#peek() !== ":") {
      if (first === null) this.#fail("expected an index, slice, name or *");
      return { kind: "index", index: first };
    }
    this.#pos++; // :
    this.#skipSpace();
    const end = this.#int();
    this.#skipSpace();
    let step: number | null = null;
    if (this.#peek() === ":") {
      this.#pos++;
      this.#skipSpace();
      step = this.#int();
    }
    return { kind: "slice", start: first, end, step };
  }
}

function isObject(v: unknown): v is Record<string, unknown> {
  return typeof v === "object" && v !== null && !Array.isArray(v);
}

function applySelector(node: unknown, sel: Selector): unknown[] {
  switch (sel.kind) {
    case "name":
      return isObject(node) && Object.hasOwn(node, sel.name) ? [node[sel.name]] : [];
    case "wildcard":
      if (Array.isArray(node)) return [...node];
      if (isObject(node)) return Object.values(node);
      return [];
    case "index": {
      if (!Array.isArray(node)) return [];
      const i = sel.index < 0 ? node.length + sel.index : sel.index;
      return i >= 0 && i < node.length ? [node[i]] : [];
    }
    case "slice": {
      if (!Array.isArray(node)) return [];
      return slice(node, sel.start, sel.end, sel.step);
    }
  }
}

// RFC 9535 §2.3.4.2.2 slice semantics (Python-style, with negative
// indices and steps).
function slice(arr: unknown[], start: number | null, end: number | null, step: number | null): unknown[] {
  const len = arr.length;
  const st = step ?? 1;
  if (st === 0) return [];
  const norm = (i: number) => (i >= 0 ? i : len + i);
  const out: unknown[] = [];
  if (st > 0) {
    const lower = Math.min(Math.max(norm(start ?? 0), 0), len);
    const upper = Math.min(Math.max(norm(end ?? len), 0), len);
    for (let i = lower; i < upper; i += st) out.push(arr[i]);
  } else {
    const upper = Math.min(Math.max(norm(start ?? len - 1), -1), len - 1);
    const lower = Math.min(Math.max(norm(end ?? -len - 1), -1), len - 1);
    for (let i = upper; i > lower; i += st) out.push(arr[i]);
  }
  return out;
}

/** Pre-order: the node itself, then every descendant. */
function descendants(node: unknown): unknown[] {
  const out: unknown[] = [node];
  if (Array.isArray(node)) {
    for (const child of node) out.push(...descendants(child));
  } else if (isObject(node)) {
    for (const child of Object.values(node)) out.push(...descendants(child));
  }
  return out;
}

/**
 * Evaluates `path` against `doc` and returns every selected node, in
 * document order. Throws `JSONPathError` for a malformed or unsupported
 * path; a well-formed path that simply matches nothing returns `[]`.
 */
export function selectPath(doc: unknown, path: string): unknown[] {
  const segments = new Parser(path).parse();
  let current: unknown[] = [doc];
  for (const seg of segments) {
    const inputs = seg.descendant ? current.flatMap(descendants) : current;
    current = inputs.flatMap((node) => seg.selectors.flatMap((sel) => applySelector(node, sel)));
  }
  return current;
}
