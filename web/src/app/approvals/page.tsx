import { redirect } from "next/navigation";
import { auth } from "@/auth";
import { AppShell } from "@/components/app-shell";
import { ApprovalsClient } from "./approvals-client";

export default async function ApprovalsPage() {
  const session = await auth();
  if (!session) {
    redirect("/login");
  }
  return (
    <AppShell active="approvals">
      <ApprovalsClient />
    </AppShell>
  );
}
