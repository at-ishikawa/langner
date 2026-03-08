"use client";

import { useEffect, useState } from "react";
import { Box, Button, Spinner, Text } from "@chakra-ui/react";
import { Dialog } from "@chakra-ui/react";
import { notebookClient } from "@/lib/client";

interface PdfPreviewModalProps {
  notebookId: string;
  isOpen: boolean;
  onClose: () => void;
}

export function PdfPreviewModal({
  notebookId,
  isOpen,
  onClose,
}: PdfPreviewModalProps) {
  const [blobUrl, setBlobUrl] = useState<string | null>(null);
  const [filename, setFilename] = useState("notebook.pdf");
  const [error, setError] = useState<string | null>(null);

  // loading is true while modal is open but PDF hasn't arrived yet
  const loading = isOpen && !blobUrl && !error;

  useEffect(() => {
    if (!isOpen) return;

    let localUrl: string | null = null;

    notebookClient
      .exportNotebookPDF({ notebookId })
      .then((res) => {
        localUrl = URL.createObjectURL(
          new Blob([new Uint8Array(res.pdfContent)], { type: "application/pdf" }),
        );
        setBlobUrl(localUrl);
        if (res.filename) setFilename(res.filename);
      })
      .catch(() => setError("Failed to export PDF"));

    return () => {
      if (localUrl) URL.revokeObjectURL(localUrl);
      setBlobUrl(null);
      setError(null);
    };
  }, [isOpen, notebookId]);

  const handleDownload = () => {
    if (!blobUrl) return;
    const link = document.createElement("a");
    link.href = blobUrl;
    link.download = filename;
    link.click();
  };

  return (
    <Dialog.Root open={isOpen} onOpenChange={(e) => !e.open && onClose()}>
      <Dialog.Backdrop />
      <Dialog.Positioner>
        <Dialog.Content maxW="4xl" w="90vw" h="85vh">
          <Dialog.Header>
            <Dialog.Title>PDF Preview</Dialog.Title>
            <Dialog.CloseTrigger />
          </Dialog.Header>
          <Dialog.Body display="flex" flexDirection="column" flex="1" p={4}>
            {loading && (
              <Box display="flex" alignItems="center" justifyContent="center" flex="1" gap={3}>
                <Spinner size="lg" />
                <Text>Generating PDF…</Text>
              </Box>
            )}
            {error && (
              <Box display="flex" alignItems="center" justifyContent="center" flex="1">
                <Text color="red.500">{error}</Text>
              </Box>
            )}
            {blobUrl && (
              <iframe src={blobUrl} style={{ flex: 1, width: "100%", border: "none" }} />
            )}
          </Dialog.Body>
          <Dialog.Footer>
            <Button variant="outline" onClick={onClose}>
              Close
            </Button>
            <Button
              colorPalette="blue"
              onClick={handleDownload}
              disabled={!blobUrl}
            >
              Download
            </Button>
          </Dialog.Footer>
        </Dialog.Content>
      </Dialog.Positioner>
    </Dialog.Root>
  );
}
