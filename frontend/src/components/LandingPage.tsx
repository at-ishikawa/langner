"use client";

import { Box, Button, Flex, Heading, SimpleGrid, Text, VStack } from "@chakra-ui/react";
import NextLink from "next/link";
import type { ReactNode } from "react";
import { Card } from "./Card";

// LandingPage is the public front door shown at "/" to a logged-out visitor
// (the SessionProvider guard treats "/" as public, so it is not bounced through
// Google). It leads with the core feature — spaced-repetition vocabulary
// quizzes — and routes to the two real entry points: "Get started" → /signup
// (account creation via Google) and "Sign in" → /login. Styled through the
// Reading Room tokens so it is theme-aware and responsive like the rest of the
// app.
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
            Spaced-repetition vocabulary
          </Text>
          <Heading
            as="h1"
            fontSize={{ base: "4xl", md: "6xl" }}
            lineHeight="1.05"
            fontWeight="600"
          >
            Learn English words — and actually remember them.
          </Heading>
          <Text fontSize={{ base: "md", md: "lg" }} color="fg.muted" maxW="xl">
            Langner quizzes you on the English words you want to learn, checking that
            you know what they mean rather than how they&apos;re spelled, and brings each
            one back right before you would forget. A few minutes a day is enough.
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
            title="Checks that you understand"
            body="Langner shows you a word and asks what it means, or gives the meaning and asks for the word. It cares that you got the idea — a close paraphrase or a synonym counts, and a small typo won't fail you."
          />
          <Feature
            title="A schedule that beats forgetting"
            body="Each word is tracked on a spaced-repetition schedule, so it reappears just as it's about to slip. Open the app and answer what's due today — you never have to plan reviews."
          />
          <Feature
            title="Add words your way"
            body="Build notebooks of the words you care about — type them in, or highlight them while reading a real public-domain book. Every word becomes a private card only you quiz."
          />
        </SimpleGrid>

        {/* Spaced repetition is the engine of the app but is unfamiliar to most
            people, so explain it plainly and make the schedule concrete. */}
        <Box
          mt={{ base: 12, md: 20 }}
          maxW="2xl"
          mx="auto"
          textAlign="center"
        >
          <Text
            fontFamily="mono"
            fontSize="xs"
            letterSpacing="0.14em"
            textTransform="uppercase"
            color="brand.fg"
            mb={3}
          >
            What is spaced repetition?
          </Text>
          <Heading as="h2" fontSize={{ base: "2xl", md: "3xl" }} fontWeight="600" mb={4}>
            Review less. Remember more.
          </Heading>
          <Text color="fg.muted" fontSize={{ base: "md", md: "lg" }} lineHeight="1.7">
            Every time you get a word right, you remember it for longer — so
            there&apos;s no point drilling it again tomorrow. Langner waits a little
            longer after each success before asking again. Words you find hard come
            back soon; words you know drift far into the future. You just answer what&apos;s
            due each day, and the gaps grow on their own:
          </Text>

          <Flex
            gap={2}
            flexWrap="wrap"
            justify="center"
            align="center"
            mt={6}
            fontFamily="mono"
            fontSize={{ base: "xs", md: "sm" }}
          >
            {["Now", "in 3 days", "in 1 week", "in 1 month", "in 3 months"].map(
              (label, i) => (
                <Flex key={label} align="center" gap={2}>
                  {i > 0 && (
                    <Text aria-hidden color="fg.muted">
                      →
                    </Text>
                  )}
                  <Box
                    borderWidth="1px"
                    borderColor="border"
                    borderRadius="full"
                    px={3}
                    py={1}
                    color={i === 0 ? "brand.fg" : "fg.muted"}
                    css={{ fontVariantNumeric: "tabular-nums" }}
                  >
                    {label}
                  </Box>
                </Flex>
              ),
            )}
          </Flex>
          <Text color="fg.subtle" fontSize="xs" mt={3}>
            Each correct answer pushes the next review further out; a wrong one brings it back.
          </Text>
        </Box>

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
