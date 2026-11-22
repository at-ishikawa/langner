import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import { ConnectError, Code } from "@connectrpc/connect";
import { ReviewScheduleSettings, parseDays } from "./ReviewScheduleSettings";
import { quizClient } from "@/lib/client";

vi.mock("@/lib/client", () => ({
  quizClient: {
    getIntervalSettings: vi.fn(),
    updateIntervalSettings: vi.fn(),
  },
}));

const getMock = vi.mocked(quizClient.getIntervalSettings);
const updateMock = vi.mocked(quizClient.updateIntervalSettings);

const defaultLadder = { algorithm: "fixed", fixedIntervals: [1, 7, 30, 90, 365] };

function response(settings: { algorithm: string; fixedIntervals: number[] }, isDefault: boolean) {
  // Only the fields the component reads; the RPC types carry more.
  return { settings, isDefault, defaultSettings: defaultLadder } as never;
}

function renderSettings() {
  return render(
    <ChakraProvider value={defaultSystem}>
      <ReviewScheduleSettings />
    </ChakraProvider>,
  );
}

describe("parseDays", () => {
  it.each([
    ["1, 7, 30", [1, 7, 30]],
    [" 2,5 ,20, ", [2, 5, 20]],
    ["", []],
    ["1, seven", null],
    ["1.5, 3", null],
    ["-1, 3", null],
  ])("parses %j", (text, want) => {
    expect(parseDays(text)).toEqual(want);
  });
});

describe("ReviewScheduleSettings", () => {
  beforeEach(() => {
    getMock.mockReset();
    updateMock.mockReset();
  });

  it("shows the default schedule and says changes only affect future answers", async () => {
    getMock.mockResolvedValue(response(defaultLadder, true));
    renderSettings();

    const input = await screen.findByLabelText("Days between reviews");
    await waitFor(() => expect(input).toHaveValue("1, 7, 30, 90, 365"));
    expect(screen.getByText(/You are using the default schedule/)).toBeInTheDocument();
    expect(screen.getByText(/keep the review dates they have now/)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Use the default" })).not.toBeInTheDocument();
  });

  it("saves a fixed ladder", async () => {
    getMock.mockResolvedValue(response(defaultLadder, true));
    updateMock.mockResolvedValue(response({ algorithm: "fixed", fixedIntervals: [2, 5, 20] }, false));
    renderSettings();

    const input = await screen.findByLabelText("Days between reviews");
    await waitFor(() => expect(input).toHaveValue("1, 7, 30, 90, 365"));
    fireEvent.change(input, { target: { value: "2, 5, 20" } });
    fireEvent.click(screen.getByRole("button", { name: "Save schedule" }));

    await waitFor(() =>
      expect(updateMock).toHaveBeenCalledWith({ settings: { algorithm: "fixed", fixedIntervals: [2, 5, 20] } }),
    );
    expect(await screen.findByRole("status")).toHaveTextContent("Saved.");
    expect(screen.getByRole("button", { name: "Use the default" })).toBeInTheDocument();
    expect(screen.getByText(/The default schedule is fixed steps of 1, 7, 30, 90, 365 days/)).toBeInTheDocument();
  });

  it("saves the adaptive schedule without a ladder", async () => {
    getMock.mockResolvedValue(response(defaultLadder, true));
    updateMock.mockResolvedValue(response({ algorithm: "modified_sm2", fixedIntervals: [] }, false));
    renderSettings();

    await screen.findByLabelText("Days between reviews");
    fireEvent.click(screen.getByRole("button", { name: "Adaptive" }));
    expect(screen.queryByLabelText("Days between reviews")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Save schedule" }));

    await waitFor(() =>
      expect(updateMock).toHaveBeenCalledWith({ settings: { algorithm: "modified_sm2", fixedIntervals: [] } }),
    );
    expect(screen.getByRole("button", { name: "Adaptive" })).toHaveAttribute("aria-pressed", "true");
  });

  it("rejects days that are not whole numbers without calling the server", async () => {
    getMock.mockResolvedValue(response(defaultLadder, true));
    renderSettings();

    const input = await screen.findByLabelText("Days between reviews");
    fireEvent.change(input, { target: { value: "1, a week" } });
    fireEvent.click(screen.getByRole("button", { name: "Save schedule" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("Enter whole numbers of days");
    expect(updateMock).not.toHaveBeenCalled();
  });

  it("shows the server's validation error", async () => {
    getMock.mockResolvedValue(response(defaultLadder, true));
    updateMock.mockRejectedValue(
      new ConnectError("invalid review schedule: each interval must be longer than the one before it", Code.InvalidArgument),
    );
    renderSettings();

    const input = await screen.findByLabelText("Days between reviews");
    fireEvent.change(input, { target: { value: "7, 1" } });
    fireEvent.click(screen.getByRole("button", { name: "Save schedule" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("each interval must be longer than the one before it");
  });

  it("resets to the default", async () => {
    getMock.mockResolvedValue(response({ algorithm: "fixed", fixedIntervals: [2, 5, 20] }, false));
    updateMock.mockResolvedValue(response(defaultLadder, true));
    renderSettings();

    const input = await screen.findByLabelText("Days between reviews");
    await waitFor(() => expect(input).toHaveValue("2, 5, 20"));
    fireEvent.click(screen.getByRole("button", { name: "Use the default" }));

    await waitFor(() => expect(updateMock).toHaveBeenCalledWith({ resetToDefault: true }));
    await waitFor(() => expect(input).toHaveValue("1, 7, 30, 90, 365"));
    expect(screen.getByText(/You are using the default schedule/)).toBeInTheDocument();
  });
});
