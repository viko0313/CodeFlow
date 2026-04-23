import { redirect } from "next/navigation";
import { auth } from "@/auth";
import { AppShell } from "@/components/app-shell";
import { DashboardClient } from "./dashboard-client";

export default async function DashboardPage({
  searchParams,
}: {
  searchParams?: Promise<{ session?: string }>;
}) {
  const session = await auth();
  const params = (await searchParams) ?? {};
  if (!session) {
    redirect("/login");
  }
  return (
    <AppShell active="dashboard" activeSessionId={params.session}>
      <DashboardClient userName={session.user?.name ?? "admin"} />
    </AppShell>
  );
}
