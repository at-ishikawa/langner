"use client";

import { useState } from "react";
import Link from "next/link";
import { Box, Heading, Text } from "@chakra-ui/react";

type Tab = "vocabulary" | "etymology";

const vocabularyModes = [
  {
    href: "/quiz/start?mode=standard",
    title: "Standard",
    description: "See a word, type its meaning",
  },
  {
    href: "/quiz/start?mode=reverse",
    title: "Reverse",
    description: "See a meaning, type the word",
  },
  {
    href: "/quiz/start?mode=freeform",
    title: "Freeform",
    description: "Type any word and its meaning",
  },
];

const etymologyModes = [
  {
    href: "/quiz/etymology-start?mode=breakdown",
    title: "Breakdown",
    description: "See a word, identify its origins and meanings",
  },
  {
    href: "/quiz/etymology-start?mode=assembly",
    title: "Assembly",
    description: "See origins and meanings, type the word",
  },
  {
    href: "/quiz/etymology-start?mode=freeform",
    title: "Freeform",
    description: "Type any word and break down its origins",
  },
];

export default function QuizHubPage() {
  const [tab, setTab] = useState<Tab>("vocabulary");
  const modes = tab === "vocabulary" ? vocabularyModes : etymologyModes;

  return (
    <Box maxW="sm" mx="auto" bg="#f8f9fa" minH="100vh">
      {/* Header */}
      <Box bg="white" borderBottomWidth="1px" borderColor="#e5e7eb">
        <Box px={4} pt={2}>
          <Link href="/">
            <Text color="#2563eb" fontSize="xs">
              &lt; Home
            </Text>
          </Link>
        </Box>
        <Box px={4} pb={3} textAlign="center">
          <Heading size="md">Quiz</Heading>
        </Box>
      </Box>

      {/* Tabs */}
      <Box
        bg="white"
        borderBottomWidth="1px"
        borderColor="#e5e7eb"
        display="flex"
      >
        <Box
          flex={1}
          textAlign="center"
          py={2}
          cursor="pointer"
          onClick={() => setTab("vocabulary")}
          position="relative"
        >
          <Text
            fontSize="sm"
            fontWeight={tab === "vocabulary" ? "semibold" : "normal"}
            color={tab === "vocabulary" ? "#2563eb" : "#999"}
          >
            Vocabulary
          </Text>
          {tab === "vocabulary" && (
            <Box
              position="absolute"
              bottom={0}
              left="50%"
              transform="translateX(-50%)"
              w="60%"
              h="3px"
              borderRadius="full"
              bg="#2563eb"
            />
          )}
        </Box>
        <Box
          flex={1}
          textAlign="center"
          py={2}
          cursor="pointer"
          onClick={() => setTab("etymology")}
          position="relative"
        >
          <Text
            fontSize="sm"
            fontWeight={tab === "etymology" ? "semibold" : "normal"}
            color={tab === "etymology" ? "#2563eb" : "#999"}
          >
            Etymology
          </Text>
          {tab === "etymology" && (
            <Box
              position="absolute"
              bottom={0}
              left="50%"
              transform="translateX(-50%)"
              w="60%"
              h="3px"
              borderRadius="full"
              bg="#2563eb"
            />
          )}
        </Box>
      </Box>

      {/* Content */}
      <Box p={4} display="flex" flexDirection="column" gap={3}>
        {modes.map((mode) => (
          <Link key={mode.href} href={mode.href}>
            <Box
              p={4}
              bg="white"
              borderWidth="1px"
              borderColor="#e5e7eb"
              borderRadius="lg"
              _hover={{ bg: "gray.50" }}
              cursor="pointer"
              display="flex"
              alignItems="center"
              justifyContent="space-between"
            >
              <Box>
                <Text fontWeight="semibold" fontSize="md">
                  {mode.title}
                </Text>
                <Text fontSize="xs" color="#666">
                  {mode.description}
                </Text>
              </Box>
              <Text fontSize="sm" color="#999" flexShrink={0}>
                &rsaquo;
              </Text>
            </Box>
          </Link>
        ))}
      </Box>
    </Box>
  );
}
