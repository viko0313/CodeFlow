import { redirect } from "next/navigation";
import { auth } from "@/auth";
import { AppShell } from "@/components/app-shell";
import { DashboardClient } from "./dashboard-client";

export default async function DashboardPage() {
  const session = await auth();
  if (!session) {
    redirect("/login");
  }
  return (
    <AppShell active="dashboard">
      <DashboardClient userName={session.user?.name ?? "admin"} />
    </AppShell>
  );
}
