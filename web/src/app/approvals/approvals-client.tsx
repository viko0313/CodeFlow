"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { approveApproval, getApprovals, getTaskEvents, rejectApproval } from "@/lib/codeflow-api";
import { formatDateTime } from "@/lib/utils";

export function ApprovalsClient() {
  const queryClient = useQueryClient();
  const [reasons, setReasons] = useState<Record<string, string>>({});
  const pending = useQuery({
    queryKey: ["approvals", "pending"],
    queryFn: () => getApprovals({ status: "pending", limit: 100 }),
    refetchInterval: 3000,
  });
  const recent = useQuery({
    queryKey: ["approvals", "recent"],
    queryFn: () => getApprovals({ limit: 100 }),
    refetchInterval: 5000,
  });
  const events = useQuery({
    queryKey: ["task-events", "approval"],
    queryFn: () => getTaskEvents({ limit: 80 }),
    refetchInterval: 5000,
  });
  const approve = useMutation({
    mutationFn: (id: string) => approveApproval(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["approvals"] });
      queryClient.invalidateQueries({ queryKey: ["task-events"] });
    },
  });
  const reject = useMutation({
    mutationFn: ({ id, reason }: { id: string; reason: string }) => rejectApproval(id, reason),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["approvals"] });
      queryClient.invalidateQueries({ queryKey: ["task-events"] });
    },
  });

  return (
    <div className="mx-auto grid max-w-[1480px] gap-5 px-4 py-5">
      <section className="rounded-lg border border-[var(--line)] bg-white p-5">
        <div className="mb-4 flex items-center justify-between">
          <h1 className="text-xl font-semibold">审批中心</h1>
          <Badge>{pending.data?.length ?? 0} 条待处理</Badge>
        </div>
        <div className="grid gap-3">
          {(pending.data ?? []).map((item) => {
            const reason = (reasons[item.id] ?? "").trim();
            const rejecting = reject.isPending && reject.variables?.id === item.id;
            const approving = approve.isPending && approve.variables === item.id;
            return (
              <div key={item.id} className="rounded-lg border border-[var(--line)] p-4">
                <div className="mb-2 flex items-center justify-between gap-3">
                  <div className="min-w-0">
                    <p className="truncate text-sm font-semibold">{item.kind}</p>
                    <p className="truncate text-xs text-[var(--muted)]">{item.id}</p>
                  </div>
                  <Badge>待处理</Badge>
                </div>
                {item.command ? <Row label="命令" value={item.command} /> : null}
                {item.path ? <Row label="路径" value={item.path} /> : null}
                {item.request_id ? <Row label="请求 ID" value={item.request_id} /> : null}
                <pre className="mt-2 max-h-40 overflow-auto rounded-lg bg-[var(--terminal)] p-3 text-xs text-white">
                  {item.preview || "暂无预览。"}
                </pre>
                <div className="mt-3 grid gap-2 md:grid-cols-[1fr_auto_auto]">
                  <Input
                    placeholder="拒绝时请填写原因"
                    value={reasons[item.id] ?? ""}
                    onChange={(event) =>
                      setReasons((current) => ({
                        ...current,
                        [item.id]: event.target.value,
                      }))
                    }
                  />
                  <Button
                    variant="secondary"
                    disabled={!reason || rejecting}
                    onClick={() => reject.mutate({ id: item.id, reason })}
                  >
                    拒绝
                  </Button>
                  <Button disabled={approving} onClick={() => approve.mutate(item.id)}>
                    批准
                  </Button>
                </div>
              </div>
            );
          })}
          {!pending.data?.length ? <p className="text-sm text-[var(--muted)]">当前没有待处理审批。</p> : null}
        </div>
      </section>

      <section className="grid gap-5 lg:grid-cols-2">
        <div className="rounded-lg border border-[var(--line)] bg-white p-5">
          <h2 className="mb-3 text-lg font-semibold">最近审批</h2>
          <div className="grid gap-2">
            {(recent.data ?? []).slice(0, 20).map((item) => (
              <div key={item.id} className="rounded-lg border border-[var(--line)] p-3">
                <p className="text-xs text-[var(--muted)]">{formatDateTime(item.created_at)}</p>
                <p className="truncate text-sm font-medium">
                  {item.status} / {item.kind}
                </p>
                {item.decision_reason ? <p className="truncate text-xs text-[var(--muted)]">{item.decision_reason}</p> : null}
              </div>
            ))}
          </div>
        </div>
        <div className="rounded-lg border border-[var(--line)] bg-white p-5">
          <h2 className="mb-3 text-lg font-semibold">审批事件</h2>
          <div className="grid gap-2">
            {(events.data ?? [])
              .filter((item) => item.event_type.startsWith("approval."))
              .slice(0, 20)
              .map((item) => (
                <div key={item.id} className="rounded-lg border border-[var(--line)] p-3">
                  <p className="text-xs text-[var(--muted)]">{formatDateTime(item.created_at)}</p>
                  <p className="truncate text-sm font-medium">{item.event_type}</p>
                  <p className="truncate text-xs text-[var(--muted)]">{item.message}</p>
                </div>
              ))}
          </div>
        </div>
      </section>
    </div>
  );
}

function Row({ label, value }: { label: string; value: string }) {
  return (
    <div className="grid grid-cols-[88px_1fr] gap-2 text-xs">
      <span className="text-[var(--muted)]">{label}</span>
      <span className="truncate">{value}</span>
    </div>
  );
}
