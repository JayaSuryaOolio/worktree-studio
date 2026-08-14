import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { maybeDesktopNotify } from "./attentionNotify";
import { setNotificationsEnabled } from "./notificationPreference";

class FakeNotification {
  static permission: NotificationPermission = "granted";
  static instances: FakeNotification[] = [];
  onclick: (() => void) | null = null;
  closed = false;

  constructor(
    public title: string,
    public options?: { body?: string }
  ) {
    FakeNotification.instances.push(this);
  }
  close() {
    this.closed = true;
  }
}

beforeEach(() => {
  FakeNotification.instances = [];
  FakeNotification.permission = "granted";
  vi.stubGlobal("Notification", FakeNotification);
  localStorage.clear();
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("maybeDesktopNotify", () => {
  it("fires a notification when permission is granted and the preference is on", () => {
    maybeDesktopNotify("title", "body");
    expect(FakeNotification.instances).toHaveLength(1);
    expect(FakeNotification.instances[0].title).toBe("title");
    expect(FakeNotification.instances[0].options?.body).toBe("body");
  });

  it("does nothing when permission is not granted", () => {
    FakeNotification.permission = "denied";
    maybeDesktopNotify("title", "body");
    expect(FakeNotification.instances).toHaveLength(0);
  });

  it("does nothing when the user has turned notifications off, even with permission granted", () => {
    setNotificationsEnabled(false);
    maybeDesktopNotify("title", "body");
    expect(FakeNotification.instances).toHaveLength(0);
  });

  it("wires onClick to focus the window, run the callback, and close the notification", () => {
    const focusSpy = vi.spyOn(window, "focus").mockImplementation(() => {});
    const onClick = vi.fn();
    maybeDesktopNotify("title", "body", onClick);

    const notification = FakeNotification.instances[0];
    notification.onclick?.();

    expect(focusSpy).toHaveBeenCalled();
    expect(onClick).toHaveBeenCalled();
    expect(notification.closed).toBe(true);
    focusSpy.mockRestore();
  });
});
