import { describe, expect, it } from "vitest";
import {
  NETWORK_POSTURE,
  networkPostureLabel,
} from "../../utils/network";

describe("networkPostureLabel", () => {
  it("reports no network when the section is disabled", () => {
    expect(networkPostureLabel(true, true, false)).toBe(NETWORK_POSTURE.none);
  });

  it("reports no network when neither toggle is on", () => {
    expect(networkPostureLabel(false, false)).toBe(NETWORK_POSTURE.none);
  });

  it("reports local network only when LAN is on and internet is off", () => {
    expect(networkPostureLabel(true, false)).toBe(NETWORK_POSTURE.lan);
  });

  it("reports internet only when internet is on and LAN is off", () => {
    expect(networkPostureLabel(false, true)).toBe(NETWORK_POSTURE.internetOnly);
  });

  it("reports local network + internet when both are on", () => {
    expect(networkPostureLabel(true, true)).toBe(NETWORK_POSTURE.internet);
  });

  it("does not treat internet as implying LAN", () => {
    expect(networkPostureLabel(false, true)).not.toBe(NETWORK_POSTURE.internet);
  });
});
