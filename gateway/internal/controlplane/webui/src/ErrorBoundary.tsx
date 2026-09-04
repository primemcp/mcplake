import { Component, type ReactNode } from "react";

type Props = { children: ReactNode };
type State = { error: Error | null };

/** Catches genuine render bugs, not expected API failures — those are
 * handled per-section by each data hook's {data, loading, error, retry}
 * shape instead (see the Admin UI design doc, "Error handling"). */
export class ErrorBoundary extends Component<Props, State> {
  state: State = { error: null };

  static getDerivedStateFromError(error: Error): State {
    return { error };
  }

  render() {
    if (this.state.error) {
      return (
        <div className="flex h-screen items-center justify-center text-center p-6">
          <div>
            <p className="text-sm font-medium">Something went wrong.</p>
            <p className="text-xs text-subtle mt-1">{this.state.error.message}</p>
          </div>
        </div>
      );
    }
    return this.props.children;
  }
}
