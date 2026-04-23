import { redirect } from "next/navigation";
import { auth } from "@/auth";
import { LoginForm } from "@/components/login-form";

export default async function LoginPage() {
  const session = await auth();
  if (session) {
    redirect("/dashboard");
  }
  return (
    <main className="grid min-h-screen place-items-center px-4 py-10">
      <section className="w-full max-w-sm rounded-lg border border-[var(--line)] bg-white p-6 shadow-sm">
        <div className="mb-6">
          <p className="text-sm font-semibold text-[var(--accent-strong)]">CodeFlow</p>
          <h1 className="mt-2 text-2xl font-semibold">登录</h1>
          <p className="mt-2 text-sm text-[var(--muted)]">使用本地开发账号进入工作台。</p>
        </div>
        <LoginForm />
      </section>
    </main>
  );
}
