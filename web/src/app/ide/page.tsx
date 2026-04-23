import { redirect } from "next/navigation";
import { auth } from "@/auth";
import { AppShell } from "@/components/app-shell";
import { IdeClient } from "./ide-client";

export default async function IdePage({
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
    <AppShell active="ide" activeSessionId={params.session}>
      <IdeClient />
    </AppShell>
  );
}
