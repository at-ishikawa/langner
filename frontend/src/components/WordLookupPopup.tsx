"use client";

import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type RefObject,
} from "react";
import {
  Box,
  Button,
  Heading,
  Spinner,
  Text,
  VStack,
} from "@chakra-ui/react";
import {
  notebookClient,
  type GetNotebookDetailResponse,
  type NotebookWord,
  type WordDefinition,
} from "@/lib/client";

// LookupState is exported so callers can read e.g. `lookup !== null` to decide
// whether to render the popup, or to attach keyboard handlers.
export interface LookupState {
  word: string;
  context: string;
  storyIndex: number;
  sceneIndex: number;
  definitions: WordDefinition[];
  source: string;
  loading: boolean;
  error: string | null;
  saved: boolean;
  saving: boolean;
  savedDefinition: NotebookWord | null;
  deleting: boolean;
  deleted: boolean;
}

// useWordLookup owns the state and side effects for the tap-a-word lookup flow
// shared between the book reader and the story reader under /learn. It returns
// the state plus the handlers that wire up scene onMouseUp events and the
// save/delete/close actions rendered by <WordLookupPopup>.
export function useWordLookup(
  notebookId: string,
  data: GetNotebookDetailResponse | null,
  onDefinitionRemoved: (storyIndex: number, sceneIndex: number, expression: string) => void,
) {
  const [lookup, setLookup] = useState<LookupState | null>(null);
  const popupRef = useRef<HTMLDivElement>(null);

  // Close popup on outside click
  useEffect(() => {
    function handleClickOutside(e: MouseEvent) {
      if (popupRef.current && !popupRef.current.contains(e.target as Node)) {
        setLookup(null);
      }
    }
    document.addEventListener("mousedown", handleClickOutside);
    return () => document.removeEventListener("mousedown", handleClickOutside);
  }, []);

  const handleTextSelect = useCallback(
    (storyIndex: number, sceneIndex: number) => {
      const selection = window.getSelection();
      const selectedText = selection?.toString().trim();
      if (!selectedText || selectedText.length === 0) return;
      // Skip very long selections (likely not a word lookup)
      if (selectedText.length > 100) return;

      const range = selection?.getRangeAt(0);
      const container = range?.startContainer.parentElement;
      const context = container?.textContent?.trim() ?? "";

      const currentScene = data?.stories[storyIndex]?.scenes[sceneIndex];
      const existing = currentScene?.definitions.find(
        (d) => d.expression.toLowerCase() === selectedText.toLowerCase(),
      );

      const base: LookupState = {
        word: selectedText,
        context,
        storyIndex,
        sceneIndex,
        definitions: [],
        source: "",
        loading: true,
        error: null,
        saved: false,
        saving: false,
        savedDefinition: existing ?? null,
        deleting: false,
        deleted: false,
      };
      setLookup(base);

      notebookClient
        .lookupWord({ word: selectedText, notebookId, context })
        .then((res) => {
          setLookup((prev) => {
            if (!prev || prev.word !== selectedText) return prev;
            return {
              ...prev,
              definitions: res.definitions,
              source: res.source,
              loading: false,
            };
          });
        })
        .catch(() => {
          setLookup((prev) => {
            if (!prev || prev.word !== selectedText) return prev;
            return {
              ...prev,
              loading: false,
              error: existing ? null : "Failed to look up word",
            };
          });
        });
    },
    [notebookId, data],
  );

  const handleSaveDefinition = useCallback(
    (defIndex: number) => {
      if (!lookup || !data) return;
      const def = lookup.definitions[defIndex];
      if (!def) return;
      const story = data.stories[lookup.storyIndex];
      if (!story) return;

      setLookup((prev) => (prev ? { ...prev, saving: true } : null));

      notebookClient
        .registerDefinition({
          notebookId,
          notebookFile: story.event || "",
          sceneIndex: lookup.sceneIndex,
          expression: lookup.word,
          meaning: def.definition,
          partOfSpeech: def.partOfSpeech,
          examples: def.examples,
        })
        .then(() => {
          setLookup((prev) =>
            prev ? { ...prev, saving: false, saved: true } : null,
          );
        })
        .catch(() => {
          setLookup((prev) =>
            prev
              ? { ...prev, saving: false, error: "Failed to save definition" }
              : null,
          );
        });
    },
    [lookup, data, notebookId],
  );

  const handleDelete = useCallback(() => {
    if (!lookup || !data) return;
    const story = data.stories[lookup.storyIndex];
    if (!story) return;

    setLookup((prev) => (prev ? { ...prev, deleting: true } : null));

    notebookClient
      .deleteDefinition({
        notebookId,
        notebookFile: story.event || "",
        sceneIndex: lookup.sceneIndex,
        expression: lookup.word,
      })
      .then(() => {
        setLookup((prev) =>
          prev ? { ...prev, deleting: false, deleted: true } : null,
        );
        onDefinitionRemoved(lookup.storyIndex, lookup.sceneIndex, lookup.word);
      })
      .catch(() => {
        setLookup((prev) =>
          prev
            ? { ...prev, deleting: false, error: "Failed to delete definition" }
            : null,
        );
      });
  }, [lookup, data, notebookId, onDefinitionRemoved]);

  const close = useCallback(() => setLookup(null), []);

  return {
    lookup,
    popupRef,
    onTextSelect: handleTextSelect,
    onSaveDefinition: handleSaveDefinition,
    onDelete: handleDelete,
    onClose: close,
  };
}

export interface WordLookupPopupProps {
  lookup: LookupState;
  popupRef: RefObject<HTMLDivElement | null>;
  onSaveDefinition: (defIndex: number) => void;
  onDelete: () => void;
  onClose: () => void;
}

// WordLookupPopup renders the bottom-sheet popup that appears after the user
// selects a word in the reader. It is a pure view: all state lives in
// useWordLookup().
export function WordLookupPopup({
  lookup,
  popupRef,
  onSaveDefinition,
  onDelete,
  onClose,
}: WordLookupPopupProps) {
  return (
    <Box
      ref={popupRef}
      position="fixed"
      bottom={0}
      left={0}
      right={0}
      bg="bg.panel"
      borderTopWidth="2px"
      borderColor="blue.400"
      _dark={{ borderColor: "blue.600" }}
      p={4}
      maxH="50vh"
      overflowY="auto"
      zIndex={100}
      boxShadow="lg"
    >
      {lookup.savedDefinition && !lookup.deleted ? (
        <SavedDefinitionView
          lookup={lookup}
          onSaveDefinition={onSaveDefinition}
          onDelete={onDelete}
          onClose={onClose}
        />
      ) : lookup.deleted ? (
        <Box>
          <Text color="fg.muted" fontSize="sm">
            Definition deleted.
          </Text>
          <Button size="xs" variant="ghost" mt={2} onClick={onClose}>
            Close
          </Button>
        </Box>
      ) : (
        <NewDefinitionView
          lookup={lookup}
          onSaveDefinition={onSaveDefinition}
          onClose={onClose}
        />
      )}
    </Box>
  );
}

function SavedDefinitionView({
  lookup,
  onSaveDefinition,
  onDelete,
  onClose,
}: Omit<WordLookupPopupProps, "popupRef">) {
  const saved = lookup.savedDefinition!;
  return (
    <Box>
      <Box
        display="flex"
        alignItems="center"
        gap={2}
        mb={3}
        justifyContent="space-between"
      >
        <Box display="flex" alignItems="center" gap={2}>
          <Heading size="sm">{lookup.word}</Heading>
          <Text
            fontSize="xs"
            px={2}
            py={0.5}
            bg="green.100"
            color="green.700"
            borderRadius="full"
            _dark={{ bg: "green.900", color: "green.200" }}
          >
            Saved
          </Text>
        </Box>
        <Button size="xs" variant="ghost" onClick={onClose}>
          Close
        </Button>
      </Box>

      {saved.partOfSpeech && (
        <Text fontSize="xs" color="fg.muted" mb={1}>
          {saved.partOfSpeech}
        </Text>
      )}
      <Text fontSize="sm" fontWeight="medium" mb={1}>
        {saved.meaning || saved.definition}
      </Text>
      {saved.pronunciation && (
        <Text fontSize="xs" color="fg.muted" mb={1}>
          /{saved.pronunciation}/
        </Text>
      )}
      {saved.origin && (
        <Text fontSize="xs" color="fg.muted" mb={1}>
          Origin: {saved.origin}
        </Text>
      )}
      {saved.synonyms.length > 0 && (
        <Text fontSize="xs" color="fg.muted" mb={1}>
          Synonyms: {saved.synonyms.join(", ")}
        </Text>
      )}
      {saved.antonyms.length > 0 && (
        <Text fontSize="xs" color="fg.muted" mb={1}>
          Antonyms: {saved.antonyms.join(", ")}
        </Text>
      )}
      {saved.examples.length > 0 && (
        <Box mb={2}>
          {saved.examples.map((ex, i) => (
            <Text key={i} fontSize="xs" color="fg.muted" pl={2}>
              {ex}
            </Text>
          ))}
        </Box>
      )}

      {lookup.error && (
        <Text color="red.500" fontSize="sm" mb={2}>
          {lookup.error}
        </Text>
      )}

      <Button
        size="xs"
        colorPalette="red"
        variant="outline"
        onClick={onDelete}
        disabled={lookup.deleting}
        mb={3}
      >
        {lookup.deleting ? "Deleting..." : "Delete definition"}
      </Button>

      {lookup.loading && (
        <Box textAlign="center" py={2}>
          <Spinner size="xs" />
          <Text fontSize="xs" color="fg.muted" mt={1}>
            Loading other definitions...
          </Text>
        </Box>
      )}

      {!lookup.loading && lookup.definitions.length > 0 && (
        <Box borderTopWidth="1px" pt={3} mt={1}>
          <Text fontSize="xs" fontWeight="bold" color="fg.muted" mb={2}>
            Other definitions ({lookup.source}):
          </Text>
          <VStack align="stretch" gap={2}>
            {lookup.definitions.map((def, i) => (
              <DefinitionCard
                key={i}
                def={def}
                onSave={() => onSaveDefinition(i)}
                saving={lookup.saving}
                saved={lookup.saved}
                saveLabel="Save instead"
              />
            ))}
          </VStack>
        </Box>
      )}
    </Box>
  );
}

function NewDefinitionView({
  lookup,
  onSaveDefinition,
  onClose,
}: Omit<WordLookupPopupProps, "popupRef" | "onDelete">) {
  return (
    <Box>
      <Box
        display="flex"
        justifyContent="space-between"
        alignItems="center"
        mb={3}
      >
        <Heading size="sm">
          {lookup.word}
          {lookup.source && (
            <Text
              as="span"
              fontWeight="normal"
              fontSize="xs"
              color="fg.muted"
              ml={2}
            >
              ({lookup.source})
            </Text>
          )}
        </Heading>
        <Button size="xs" variant="ghost" onClick={onClose}>
          Close
        </Button>
      </Box>

      {lookup.loading && (
        <Box textAlign="center" py={4}>
          <Spinner size="sm" />
          <Text fontSize="sm" color="fg.muted" mt={2}>
            Looking up...
          </Text>
        </Box>
      )}

      {lookup.error && (
        <Text color="red.500" fontSize="sm">
          {lookup.error}
        </Text>
      )}

      {lookup.saved && (
        <Text
          color="green.600"
          _dark={{ color: "green.300" }}
          fontSize="sm"
          mb={2}
        >
          Definition saved.
        </Text>
      )}

      {!lookup.loading &&
        lookup.definitions.length === 0 &&
        !lookup.error && (
          <Text color="fg.muted" fontSize="sm">
            No definitions found.
          </Text>
        )}

      <VStack align="stretch" gap={3}>
        {lookup.definitions.map((def, i) => (
          <DefinitionCard
            key={i}
            def={def}
            onSave={() => onSaveDefinition(i)}
            saving={lookup.saving}
            saved={lookup.saved}
            saveLabel="Save"
          />
        ))}
      </VStack>
    </Box>
  );
}

function DefinitionCard({
  def,
  onSave,
  saving,
  saved,
  saveLabel,
}: {
  def: WordDefinition;
  onSave: () => void;
  saving: boolean;
  saved: boolean;
  saveLabel: string;
}) {
  return (
    <Box p={3} borderWidth="1px" borderRadius="md" fontSize="sm">
      {def.partOfSpeech && (
        <Text fontSize="xs" color="fg.muted" mb={1}>
          {def.partOfSpeech}
        </Text>
      )}
      <Text mb={1}>{def.definition}</Text>
      {def.pronunciation && (
        <Text fontSize="xs" color="fg.muted" mb={1}>
          /{def.pronunciation}/
        </Text>
      )}
      {def.examples.length > 0 && (
        <Box mt={1}>
          {def.examples.map((ex, j) => (
            <Text key={j} fontSize="xs" color="fg.muted" pl={2}>
              {ex}
            </Text>
          ))}
        </Box>
      )}
      {def.origin && (
        <Text fontSize="xs" color="fg.muted" mt={1}>
          Origin: {def.origin}
        </Text>
      )}
      {def.synonyms.length > 0 && (
        <Text fontSize="xs" color="fg.muted" mt={1}>
          Synonyms: {def.synonyms.join(", ")}
        </Text>
      )}
      {def.antonyms.length > 0 && (
        <Text fontSize="xs" color="fg.muted" mt={1}>
          Antonyms: {def.antonyms.join(", ")}
        </Text>
      )}
      {!saved && (
        <Button
          size="xs"
          colorPalette="blue"
          mt={2}
          onClick={onSave}
          disabled={saving}
        >
          {saving ? "Saving..." : saveLabel}
        </Button>
      )}
    </Box>
  );
}
