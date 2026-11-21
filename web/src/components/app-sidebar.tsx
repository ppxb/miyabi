import { useScrollFade } from '@animate-ui/react/primitives/effects/scroll-fade'
import { Link } from '@tanstack/react-router'
import { useRef } from 'react'

import { AppNavigation } from '@/components/app-navigation'
import {
  Sidebar,
  SidebarContent,
  SidebarGroup,
  SidebarGroupContent,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  useSidebar
} from '@/components/ui/sidebar'

export function AppSidebar(props: React.ComponentProps<typeof Sidebar>) {
  const { setOpenMobile } = useSidebar()
  const contentRef = useRef<HTMLDivElement>(null)
  useScrollFade(contentRef)
  return (
    <Sidebar {...props}>
      <SidebarHeader>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton render={<Link to="/" onClick={() => setOpenMobile(false)} />}>
              <span className="text-base font-semibold">Miyabi</span>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarHeader>
      <SidebarContent ref={contentRef}>
        <SidebarGroup>
          <SidebarGroupContent>
            <AppNavigation />
          </SidebarGroupContent>
        </SidebarGroup>
      </SidebarContent>
    </Sidebar>
  )
}
