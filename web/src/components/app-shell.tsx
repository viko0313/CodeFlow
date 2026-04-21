import Link from "next/link";
import { signOut } from "@/auth";
import { Button } from "@/components/ui/button";

export function AppShell({ children, active }: { children: React.ReactNode; active: "dashboard" | "ide" }) {
  return (
    <main className="min-h-screen">
      <header className="sticky top-0 z-30 border-b border-[var(--line)] bg-white/95 backdrop-blur">
        <div className="mx-auto flex max-w-[1480px] items-center justify-between px-4 py-3">
          <Link href="/dashboard" className="flex min-w-0 items-center gap-3">
            <span className="grid h-9 w-9 place-items-center rounded-lg bg-[var(--terminal)] text-sm font-bold text-white">
              CF
            </span>
            <span className="truncate text-base font-semibold">CodeFlow</span>
          </Link>
          <nav className="flex items-center gap-2">
            <Button asChild variant={active === "dashboard" ? "primary" : "ghost"} size="sm">
              <Link href="/dashboard">Dashboard</Link>
            </Button>
            <Button asChild variant={active === "ide" ? "primary" : "ghost"} size="sm">
              <Link href="/ide">IDE</Link>
            </Button>
            <form
              action={async () => {
                "use server";
                await signOut({ redirectTo: "/login" });
              }}
            >
              <Button variant="secondary" size="sm" type="submit">
                Sign out
              </Button>
            </form>
          </nav>
        </div>
      </header>
      {children}
    </main>
  );
}
