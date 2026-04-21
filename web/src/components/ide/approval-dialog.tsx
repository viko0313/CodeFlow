"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import type { ClientMessage } from "@/lib/types";
import { useUiStore } from "@/stores/use-ui-store";

export function ApprovalDialog({ send }: { send: (message: ClientMessage) => boolean }) {
  const approval = useUiStore((store) => store.pendingApproval);
  const setPendingApproval = useUiStore((store) => store.setPendingApproval);
  const [reason, setReason] = useState("");
  const [error, setError] = useState<string | undefined>();

  function decide(allowed: boolean, fallbackReason?: string) {
    if (!approval) return;
    const finalReason = (allowed ? "" : reason.trim() || fallbackReason || "").trim();
    if (!allowed && !finalReason) {
      setError("Reject reason is required.");
      return;
    }
    send({
      type: "permission.decide",
      approval_id: approval.approval_id,
      operation_id: approval.operation_id,
      allowed,
      reason: finalReason || undefined,
      request_id: approval.request_id,
    });
    setReason("");
    setError(undefined);
    setPendingApproval(undefined);
  }

  return (
    <Dialog
      open={Boolean(approval)}
      onOpenChange={(open) => {
        if (!open) decide(false, "dismissed in approval dialog");
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Approve {approval?.kind ?? "operation"}</DialogTitle>
          <DialogDescription>
            Review the preview from the local CodeFlow server before continuing.
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-3 text-sm">
          {approval?.command ? <Row label="Command" value={approval.command} /> : null}
          {approval?.path ? <Row label="Path" value={approval.path} /> : null}
          {approval?.risk ? <Row label="Risk" value={approval.risk} /> : null}
          {approval?.timeout ? <Row label="Timeout" value={approval.timeout} /> : null}
          <pre className="max-h-72 overflow-auto rounded-lg bg-[var(--terminal)] p-3 text-xs text-white">
            {approval?.preview ?? "No preview supplied."}
          </pre>
          <Input
            placeholder="Reason required when denying"
            value={reason}
            onChange={(event) => {
              setReason(event.target.value);
              if (error) setError(undefined);
            }}
          />
          {error ? <p className="text-xs text-[var(--danger)]">{error}</p> : null}
        </div>
        <div className="flex justify-end gap-2">
          <Button variant="secondary" onClick={() => decide(false)}>
            Deny
          </Button>
          <Button onClick={() => decide(true)}>Approve</Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}

function Row({ label, value }: { label: string; value: string }) {
  return (
    <div className="grid grid-cols-[90px_1fr] gap-3">
      <span className="text-[var(--muted)]">{label}</span>
      <span className="break-all font-medium">{value}</span>
    </div>
  );
}
