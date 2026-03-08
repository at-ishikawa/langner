"use client";

import Link from "next/link";
import { Box, Heading, Text } from "@chakra-ui/react";

const features = [
  {
    href: "/quiz",
    title: "Quiz",
    description: "Practice vocabulary with spaced repetition",
    icon: "🧠",
  },
  {
    href: "/notebooks",
    title: "Notebooks",
    description: "Browse stories, scenes, and vocabulary",
    icon: "📖",
  },
];

export default function HomePage() {
  return (
    <Box p={6} maxW="sm" mx="auto">
      <Box textAlign="center" mb={8}>
        <Heading size="xl" mb={2}>
          Langner
        </Heading>
        <Text color="fg.muted">What would you like to do?</Text>
      </Box>

      <Box display="flex" flexDirection="column" gap={4}>
        {features.map((feature) => (
          <Link key={feature.href} href={feature.href}>
            <Box
              p={5}
              borderWidth="1px"
              borderRadius="lg"
              _hover={{ bg: "bg.muted", borderColor: "blue.400" }}
              cursor="pointer"
              display="flex"
              alignItems="center"
              gap={4}
            >
              <Text fontSize="2xl">{feature.icon}</Text>
              <Box>
                <Text fontWeight="semibold" fontSize="lg">
                  {feature.title}
                </Text>
                <Text fontSize="sm" color="fg.muted">
                  {feature.description}
                </Text>
              </Box>
            </Box>
          </Link>
        ))}
      </Box>
    </Box>
  );
}
