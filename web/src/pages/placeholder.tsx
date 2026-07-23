export function PlaceholderPage({
  title,
  description,
}: {
  title: string
  description: string
}) {
  return (
    <div className="mx-auto max-w-2xl">
      <h1 className="text-2xl font-semibold tracking-tight">{title}</h1>
      <p className="mt-2 text-sm text-muted-foreground">{description}</p>
      <div className="mt-6 rounded-lg border border-dashed border-border bg-muted/40 p-8 text-center text-sm text-muted-foreground">
        Coming in a later milestone. Shell navigation is ready.
      </div>
    </div>
  )
}
