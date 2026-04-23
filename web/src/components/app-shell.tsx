import Link from "next/link";
import { signOut } from "@/auth";
import { Button } from "@/components/ui/button";

export function AppShell({
  children,
  active,
  activeSessionId,
}: {
  children: React.ReactNode;
  active: "dashboard" | "ide" | "approvals";
  activeSessionId?: string;
}) {
  const dashboardHref = withSession("/dashboard", activeSessionId);
  const ideHref = withSession("/ide", activeSessionId);
  const approvalsHref = withSession("/approvals", activeSessionId);

  return (
    <main className="min-h-screen">
      <header className="sticky top-0 z-30 border-b border-[var(--line)] bg-white/95 backdrop-blur">
        <div className="mx-auto flex max-w-[1480px] items-center justify-between px-4 py-3">
          <Link href={dashboardHref} className="flex min-w-0 items-center gap-3">
            <span className="grid h-9 w-9 place-items-center rounded-lg bg-[var(--terminal)] text-sm font-bold text-white">
              CF
            </span>
            <span className="truncate text-base font-semibold">CodeFlow</span>
          </Link>
          <nav className="flex items-center gap-2">
            <Button asChild variant={active === "dashboard" ? "primary" : "ghost"} size="sm">
              <Link href={dashboardHref}>概览</Link>
            </Button>
            <Button asChild variant={active === "ide" ? "primary" : "ghost"} size="sm">
              <Link href={ideHref}>IDE</Link>
            </Button>
            <Button asChild variant={active === "approvals" ? "primary" : "ghost"} size="sm">
              <Link href={approvalsHref}>审批中心</Link>
            </Button>
            <form
              action={async () => {
                "use server";
                await signOut({ redirectTo: "/login" });
              }}
            >
              <Button variant="secondary" size="sm" type="submit">
                退出登录
              </Button>
            </form>
          </nav>
        </div>
      </header>
      {children}
    </main>
  );
}

function withSession(path: string, sessionId?: string) {
  return sessionId ? `${path}?session=${encodeURIComponent(sessionId)}` : path;
}
