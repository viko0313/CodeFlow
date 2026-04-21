import { cn } from "@/lib/utils";

export function Badge({ className, children }: { className?: string; children: React.ReactNode }) {
  return (
    <span
      className={cn(
        "inline-flex items-center rounded-md border border-[var(--line)] bg-white px-2 py-1 text-xs font-medium text-[var(--muted)]",
        className,
      )}
    >
      {children}
    </span>
  );
}
