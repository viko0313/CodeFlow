import { redirect } from "next/navigation";
import { auth } from "@/auth";
import { AppShell } from "@/components/app-shell";
import { ApprovalsClient } from "./approvals-client";

export default async function ApprovalsPage({
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
    <AppShell active="approvals" activeSessionId={params.session}>
      <ApprovalsClient />
    </AppShell>
  );
}
