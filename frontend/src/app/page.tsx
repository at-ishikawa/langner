"use client";

import Link from "next/link";
import { Box, Heading, Text } from "@chakra-ui/react";

const features = [
  {
    href: "/learn",
    title: "Learn",
    description: "Read notebooks and browse vocabulary and etymology",
    icon: "L",
  },
  {
    href: "/quiz",
    title: "Quiz",
    description: "Practice vocabulary and etymology with spaced repetition",
    icon: "Q",
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
              bg="white"
              _dark={{ bg: "gray.800" }}
              borderWidth="1px"
              borderColor="gray.200"
              _hover={{ bg: "bg.muted", borderColor: "blue.400" }}
              borderRadius="lg"
              cursor="pointer"
              display="flex"
              alignItems="center"
              gap={4}
            >
              <Box
                w="48px"
                h="48px"
                borderRadius="10px"
                bg="blue.50"
                _dark={{ bg: "blue.900" }}
                display="flex"
                alignItems="center"
                justifyContent="center"
                flexShrink={0}
              >
                <Text fontSize="xl" fontWeight="bold" color="blue.600" _dark={{ color: "blue.300" }}>
                  {feature.icon}
                </Text>
              </Box>
              <Box flex="1">
                <Text fontWeight="semibold" fontSize="lg">
                  {feature.title}
                </Text>
                <Text fontSize="sm" color="fg.muted">
                  {feature.description}
                </Text>
              </Box>
              <Text fontSize="md" color="gray.500" _dark={{ color: "gray.400" }} flexShrink={0}>
                &rsaquo;
              </Text>
            </Box>
          </Link>
        ))}
      </Box>
    </Box>
  );
}
