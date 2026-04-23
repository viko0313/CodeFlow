import { afterEach, describe, expect, it } from "vitest";
import { useUiStore } from "@/stores/use-ui-store";

describe("useUiStore", () => {
  afterEach(() => {
    window.localStorage.clear();
    useUiStore.getState().resetForTests();
  });

  it("persists ide state by session", () => {
    useUiStore.getState().setActiveSessionId("s1");
    useUiStore.getState().setPromptDraft("s1", "summarize the repo");
    useUiStore.getState().setCommandDraft("s1", "npm test");
    useUiStore.getState().setEditor("s1", "README.md", "# Draft");
    useUiStore.getState().setLatestDiff("s1", {
      path: "README.md",
      preview: "@@ -1 +1 @@",
    });
    useUiStore.getState().setPaneSizes("s1", { primary: 62, diffPreviewHeight: 180, sidebarWidth: 280 });

    const snapshot = JSON.parse(window.localStorage.getItem("codeflow-ui") ?? "{}");
    const session = snapshot.state?.ideBySession?.s1;

    expect(session.promptDraft).toBe("summarize the repo");
    expect(session.commandDraft).toBe("npm test");
    expect(session.editorPath).toBe("README.md");
    expect(session.editorBuffer).toBe("# Draft");
    expect(session.latestDiff).toEqual({ path: "README.md", preview: "@@ -1 +1 @@" });
    expect(session.paneSizes).toEqual({ primary: 62, diffPreviewHeight: 180, sidebarWidth: 280 });
  });
});
