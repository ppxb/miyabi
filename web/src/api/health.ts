import { useQuery } from "@tanstack/react-query"

type Health = {
  status: "ok"
}

async function fetchHealth(): Promise<Health> {
  const response = await fetch("/api/health")
  if (!response.ok) {
    throw new Error(`health request failed with status ${response.status}`)
  }
  return response.json() as Promise<Health>
}

export function useHealth() {
  return useQuery({
    queryKey: ["health"],
    queryFn: fetchHealth,
  })
}
