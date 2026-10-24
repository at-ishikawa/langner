"use client";

import { Box, Button, Flex, Heading, SimpleGrid, Text, VStack } from "@chakra-ui/react";
import NextLink from "next/link";
import type { ReactNode } from "react";
import { Card } from "./Card";

// LandingPage is the public front door shown at "/" to a logged-out visitor
// (the SessionProvider guard treats "/" as public, so it is not bounced through
// Google). It explains what Langner is and routes to the two real entry points:
// "Get started" → /signup (account creation via Google) and "Sign in" → /login.
// Styled through the Reading Room tokens so it is theme-aware and responsive like
// the rest of the app.
export function LandingPage() {
  return (
    <Box as="main">
      <Box maxW="4xl" mx="auto" px={4} py={{ base: 12, md: 20 }}>
        <VStack gap={6} textAlign="center" maxW="2xl" mx="auto">
          <Text
            fontFamily="mono"
            fontSize="xs"
            letterSpacing="0.14em"
            textTransform="uppercase"
            color="brand.fg"
          >
            Learn English by reading
          </Text>
          <Heading
            as="h1"
            fontSize={{ base: "4xl", md: "6xl" }}
            lineHeight="1.05"
            fontWeight="600"
          >
            Turn the books you read into the words you keep.
          </Heading>
          <Text fontSize={{ base: "md", md: "lg" }} color="fg.muted" maxW="xl">
            Langner is a quiet reading room for building English vocabulary. Read real
            books, highlight the words you want to remember, and let spaced-repetition
            quizzes bring them back right before you forget.
          </Text>
          <Flex gap={3} flexWrap="wrap" justify="center" pt={2}>
            <Button asChild colorPalette="brand" size="lg">
              <NextLink href="/signup">Get started</NextLink>
            </Button>
            <Button asChild variant="outline" size="lg">
              <NextLink href="/login">Sign in</NextLink>
            </Button>
          </Flex>
        </VStack>

        <SimpleGrid columns={{ base: 1, md: 3 }} gap={4} mt={{ base: 12, md: 20 }}>
          <Feature
            title="Read real books"
            body="Open public-domain books and stories, and read them in a calm, typeset reader built for study."
          />
          <Feature
            title="Quiz what you highlight"
            body="Every word you highlight becomes a private flashcard. Recognition and recall quizzes grade on meaning, not spelling."
          />
          <Feature
            title="Remember for good"
            body="A spaced-repetition schedule surfaces each word just as it is about to slip, so a short daily session is enough."
          />
        </SimpleGrid>

        <Box textAlign="center" mt={{ base: 12, md: 16 }}>
          <Text color="fg.muted" fontSize="sm">
            Already have an account?{" "}
            <Button
              asChild
              variant="plain"
              size="sm"
              color="brand.fg"
              h="auto"
              py={0}
              textDecoration="underline"
            >
              <NextLink href="/login">Sign in</NextLink>
            </Button>
          </Text>
        </Box>
      </Box>
    </Box>
  );
}

function Feature({ title, body }: { title: ReactNode; body: ReactNode }) {
  return (
    <Card px={6} py={6} h="100%">
      <VStack align="start" gap={2}>
        <Heading as="h2" size="md" fontWeight="600">
          {title}
        </Heading>
        <Text color="fg.muted" fontSize="sm" lineHeight="1.6">
          {body}
        </Text>
      </VStack>
    </Card>
  );
}
