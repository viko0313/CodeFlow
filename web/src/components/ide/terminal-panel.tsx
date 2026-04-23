"use client";

import { Terminal } from "@xterm/xterm";
import { useEffect, useRef } from "react";

export function TerminalPanel({ output }: { output: string }) {
  const hostRef = useRef<HTMLDivElement | null>(null);
  const terminalRef = useRef<Terminal | null>(null);

  useEffect(() => {
    if (!hostRef.current || terminalRef.current) return;
    const terminal = new Terminal({
      cursorBlink: true,
      convertEol: true,
      fontFamily: "Consolas, Menlo, monospace",
      fontSize: 13,
      theme: {
        background: "#171a1d",
        foreground: "#f6f7f9",
        cursor: "#0f9f8f",
      },
    });
    terminal.open(hostRef.current);
    terminal.writeln("CodeFlow 终端已就绪。");
    terminalRef.current = terminal;
    return () => {
      terminal.dispose();
      terminalRef.current = null;
    };
  }, []);

  useEffect(() => {
    if (!terminalRef.current) return;
    terminalRef.current.clear();
    terminalRef.current.writeln("CodeFlow 终端已就绪。");
    if (output.trim()) {
      terminalRef.current.write(output.replace(/\n/g, "\r\n"));
    }
  }, [output]);

  return <div ref={hostRef} className="min-h-0 overflow-hidden" />;
}
