// A thin indeterminate progress bar that sits on top of a table or card.

export function LoadingBar({ loading }: { loading: boolean }) {
  if (!loading) return null
  return (
    <div className="h-0.5 w-full overflow-hidden bg-border/30">
      <div className="h-full w-1/3 animate-loading-bar rounded-sm bg-primary/60" />
    </div>
  )
}
