import { redirect } from "next/navigation";
import { auth } from "@/auth";
import { AppShell } from "@/components/app-shell";
import { IdeClient } from "./ide-client";

export default async function IdePage() {
  const session = await auth();
  if (!session) {
    redirect("/login");
  }
  return (
    <AppShell active="ide">
      <IdeClient />
    </AppShell>
  );
}
