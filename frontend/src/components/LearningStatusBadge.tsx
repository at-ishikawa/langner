import { Badge } from "@chakra-ui/react";

const statusConfig: Record<string, { label: string; colorPalette: string }> = {
  "": { label: "Learning", colorPalette: "gray" },
  misunderstood: { label: "Misunderstood", colorPalette: "red" },
  understood: { label: "Understood", colorPalette: "yellow" },
  usable: { label: "Usable", colorPalette: "blue" },
  intuitive: { label: "Intuitive", colorPalette: "green" },
};

export function LearningStatusBadge({ status }: { status: string }) {
  const config = statusConfig[status] ?? statusConfig[""];
  return <Badge colorPalette={config.colorPalette}>{config.label}</Badge>;
}
