"use client";

import { forwardRef } from "react";
import { Box, Button, Input, Text } from "@chakra-ui/react";

type Props = {
  label: string;
  value: string;
  onChange: (value: string) => void;
  onSubmit: () => void;
  onSkip?: () => void;
  onKeyDown?: (e: React.KeyboardEvent) => void;
  placeholder?: string;
  submitLabel?: string;
  stickySubmit?: boolean;
};

export const AnswerInput = forwardRef<HTMLInputElement, Props>(
  function AnswerInput(
    {
      label,
      value,
      onChange,
      onSubmit,
      onSkip,
      onKeyDown,
      placeholder,
      submitLabel = "Submit",
      stickySubmit = false,
    },
    ref,
  ) {
    return (
      <>
        <Box>
          <Text fontWeight="medium" mb={1}>
            {label}
          </Text>
          <Input
            ref={ref}
            value={value}
            onChange={(e) => onChange(e.target.value)}
            onKeyDown={onKeyDown}
            placeholder={placeholder}
            size="lg"
          />
        </Box>

        <Button
          colorPalette="blue"
          onClick={onSubmit}
          disabled={!value.trim()}
          size="lg"
          {...(stickySubmit ? { position: "sticky", bottom: 4 } : {})}
        >
          {submitLabel}
        </Button>

        {onSkip && (
          <Button variant="outline" onClick={onSkip} size="lg">
            Don&apos;t Know
          </Button>
        )}
      </>
    );
  },
);
