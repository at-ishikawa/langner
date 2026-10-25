import { describe, it, expect, vi, afterEach } from "vitest";
import { createRef } from "react";
import { render, screen } from "@testing-library/react";
import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import { WordLookupPopup, type LookupState } from "./WordLookupPopup";

// useSession is the only data source the popup reads to decide whether the user
// has an LLM key; a mutable ref lets each test pick the key state.
const { sessionUser } = vi.hoisted(() => ({
  sessionUser: { current: null as { credentials: unknown[] } | null },
}));
vi.mock("./SessionProvider", () => ({
  useSession: () => ({ user: sessionUser.current, loading: false, refresh: vi.fn() }),
}));

vi.mock("next/link", () => ({
  default: ({ children, ...props }: { children: React.ReactNode; href: string }) => (
    <a {...props}>{children}</a>
  ),
}));

function emptyLookup(): LookupState {
  return {
    word: "idiom",
    context: "a common idiom",
    storyIndex: 0,
    sceneIndex: 0,
    definitions: [],
    suggestions: [],
    source: "",
    loading: false,
    error: null,
    saved: false,
    saving: false,
    savedDefinition: null,
    deleting: false,
    deleted: false,
  };
}

function renderPopup() {
  return render(
    <ChakraProvider value={defaultSystem}>
      <WordLookupPopup
        lookup={emptyLookup()}
        popupRef={createRef<HTMLDivElement>()}
        onSaveDefinition={vi.fn()}
        onSaveMeaning={vi.fn()}
        onDelete={vi.fn()}
        onClose={vi.fn()}
      />
    </ChakraProvider>,
  );
}

describe("WordLookupPopup empty state", () => {
  afterEach(() => {
    sessionUser.current = null;
    vi.clearAllMocks();
  });

  it("points a keyless user at Settings instead of 'No definitions found'", () => {
    sessionUser.current = { credentials: [] };
    renderPopup();
    expect(screen.queryByText("No definitions found.")).not.toBeInTheDocument();
    expect(screen.getByText(/add an OpenAI or Gemini API key/i)).toBeInTheDocument();
    const link = screen.getByRole("link", { name: /Add a key in Settings/i });
    expect(link).toHaveAttribute("href", "/settings");
  });

  it("shows 'No definitions found' when the user already has a key", () => {
    sessionUser.current = { credentials: [{ provider: "openai" }] };
    renderPopup();
    expect(screen.getByText("No definitions found.")).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: /Settings/i })).not.toBeInTheDocument();
  });
});
