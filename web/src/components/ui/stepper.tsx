import type { HTMLAttributes } from 'react'
import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from 'react'

import * as Stepperize from '@stepperize/react'

import { cn } from 'cn'

// Types
type StepperOrientation = 'horizontal' | 'vertical'
type StepState = 'active' | 'completed' | 'inactive' | 'loading'
type StepIndicators = {
  active?: React.ReactNode
  completed?: React.ReactNode
  inactive?: React.ReactNode
  loading?: React.ReactNode
}

type StepDefinition = {
  id: string
  title?: string
  description?: string
  icon?: React.ReactElement
}

type DefinedStepper = Stepperize.StepperDefinition<readonly StepDefinition[]>
type StepperInstance = Stepperize.Stepper<readonly StepDefinition[]>

interface StepperContextValue {
  stepper: StepperInstance
  steps: StepDefinition[]
  orientation: StepperOrientation
  configOrientation: StepperOrientation
  responsive?: boolean
  registerTrigger: (node: HTMLButtonElement | null, remove?: boolean) => void
  triggerNodes: HTMLButtonElement[]
  focusNext: (currentIdx: number) => void
  focusPrev: (currentIdx: number) => void
  focusFirst: () => void
  focusLast: () => void
  indicators: StepIndicators
}

interface StepItemContextValue {
  step: StepDefinition
  index: number
  state: StepState
  isDisabled: boolean
  isLoading: boolean
}

const StepperContext = createContext<StepperContextValue | undefined>(undefined)

const StepItemContext = createContext<StepItemContextValue | undefined>(undefined)

function useStepper() {
  const ctx = useContext(StepperContext)

  if (!ctx) throw new Error('useStepper must be used within a Stepper')

  return ctx
}

function useStepItem() {
  const ctx = useContext(StepItemContext)

  if (!ctx) throw new Error('useStepItem must be used within a StepperItem')

  return ctx
}

interface StepperProps extends HTMLAttributes<HTMLDivElement> {
  steps: StepDefinition[]
  defaultValue?: string
  orientation?: StepperOrientation
  responsive?: boolean
  indicators?: StepIndicators
  value?: string
  onValueChange?: (value: string) => void
}

function Stepper({
  steps,
  defaultValue,
  orientation = 'horizontal',
  responsive = false,
  className,
  children,
  indicators = {},
  value,
  onValueChange,
  ...props
}: StepperProps) {
  // Define stepper once — steps are expected to be stable references
  const [stepperDef] = useState<DefinedStepper>(() => {
    const define = Stepperize.defineStepper as unknown as (
      stepList: readonly StepDefinition[]
    ) => DefinedStepper
    return define(steps)
  })

  const stepper = stepperDef.useStepper({
    defaultStep: defaultValue || steps[0]?.id,
    step: value,
    onStepChange: id => onValueChange?.(id)
  })

  const [triggerNodes, setTriggerNodes] = useState<HTMLButtonElement[]>([])

  // Track viewport breakpoint (tailwind md = 768px). If `responsive` is true
  // and the configured orientation is horizontal, switch to vertical on
  // Viewport width smaller than md.
  const [isMdUp, setIsMdUp] = useState<boolean>(() =>
    typeof window !== 'undefined' ? window.matchMedia('(min-width: 768px)').matches : true
  )

  useEffect(() => {
    if (!responsive) return

    const mql = window.matchMedia('(min-width: 768px)')
    const handler = (e: MediaQueryListEvent | MediaQueryList) =>
      setIsMdUp('matches' in e ? e.matches : mql.matches)

    if ('addEventListener' in mql) {
      // modern browsers

      mql.addEventListener('change', handler)
    } else {
      // fallback
      // @ts-expect-error - legacy
      mql.addListener(handler)
    }

    return () => {
      if ('removeEventListener' in mql) {
        mql.removeEventListener('change', handler)
      } else {
        // @ts-expect-error - legacy
        mql.removeListener(handler)
      }
    }
  }, [responsive])

  // Register/unregister triggers
  const registerTrigger = useCallback((node: HTMLButtonElement | null, remove = false) => {
    setTriggerNodes(prev => {
      if (!node) return prev

      if (remove) return prev.filter(n => n !== node)

      return prev.includes(node) ? prev : [...prev, node]
    })
  }, [])

  // Keyboard navigation logic
  const focusNext = useCallback(
    (currentIdx: number) => triggerNodes[(currentIdx + 1) % triggerNodes.length]?.focus(),
    [triggerNodes]
  )

  const focusPrev = useCallback(
    (currentIdx: number) =>
      triggerNodes[(currentIdx - 1 + triggerNodes.length) % triggerNodes.length]?.focus(),
    [triggerNodes]
  )

  const focusFirst = useCallback(() => triggerNodes[0]?.focus(), [triggerNodes])

  const focusLast = useCallback(
    () => triggerNodes[triggerNodes.length - 1]?.focus(),
    [triggerNodes]
  )

  // Determine effective orientation when responsive behavior is enabled.
  const effectiveOrientation: StepperOrientation = useMemo(() => {
    if (responsive && orientation === 'horizontal') {
      return isMdUp ? 'horizontal' : 'vertical'
    }

    return orientation
  }, [responsive, orientation, isMdUp])

  // Context value
  const contextValue = useMemo<StepperContextValue>(
    () => ({
      stepper,
      steps,
      orientation: effectiveOrientation,
      configOrientation: orientation,
      responsive,
      registerTrigger,
      focusNext,
      focusPrev,
      focusFirst,
      focusLast,
      triggerNodes,
      indicators
    }),
    [
      stepper,
      steps,
      effectiveOrientation,
      orientation,
      responsive,
      registerTrigger,
      focusNext,
      focusPrev,
      focusFirst,
      focusLast,
      triggerNodes,
      indicators
    ]
  )

  return (
    <StepperContext.Provider value={contextValue}>
      <div
        role="tablist"
        aria-orientation={effectiveOrientation}
        data-slot="stepper"
        className={cn('w-full', className)}
        data-orientation={effectiveOrientation}
        {...props}
      >
        {children}
      </div>
    </StepperContext.Provider>
  )
}

interface StepperItemProps extends React.HTMLAttributes<HTMLDivElement> {
  stepId: string
  completed?: boolean
  disabled?: boolean
  loading?: boolean
}

function StepperItem({
  stepId,
  completed = false,
  disabled = false,
  loading = false,
  className,
  children,
  ...props
}: StepperItemProps) {
  const { stepper, steps } = useStepper()
  const stepIndex = steps.findIndex(s => s.id === stepId)
  const currentIndex = stepper.index
  const step = steps.find(s => s.id === stepId)!

  const state: StepState =
    completed || stepIndex < currentIndex
      ? 'completed'
      : currentIndex === stepIndex
        ? 'active'
        : 'inactive'

  const isLoading = loading && currentIndex === stepIndex

  return (
    <StepItemContext.Provider
      value={{ step, index: stepIndex, state, isDisabled: disabled, isLoading }}
    >
      <div
        data-slot="stepper-item"
        className={cn(
          'group/step flex items-center justify-center not-last:flex-1 group-data-[orientation=horizontal]/stepper-nav:flex-row group-data-[orientation=vertical]/stepper-nav:flex-col',
          className
        )}
        data-state={state}
        {...(isLoading ? { 'data-loading': true } : {})}
        {...props}
      >
        {children}
      </div>
    </StepItemContext.Provider>
  )
}

interface StepperTriggerProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  asChild?: boolean
}

function StepperTrigger({
  asChild = false,
  className,
  children,
  tabIndex,
  ...props
}: StepperTriggerProps) {
  const { state, isLoading } = useStepItem()
  const { stepper, registerTrigger, triggerNodes, focusNext, focusPrev, focusFirst, focusLast } =
    useStepper()

  const { step, isDisabled } = useStepItem()
  const isSelected = stepper.id === step.id
  const id = `stepper-tab-${step.id}`
  const panelId = `stepper-panel-${step.id}`

  // Register this trigger via callback ref for correct mount/unmount handling
  const btnRef = useRef<HTMLButtonElement | null>(null)

  const triggerRef = useCallback(
    (node: HTMLButtonElement | null) => {
      if (node) {
        btnRef.current = node
        registerTrigger(node)
      } else if (btnRef.current) {
        registerTrigger(btnRef.current, true)
        btnRef.current = null
      }
    },
    [registerTrigger]
  )

  // Find our index among triggers for navigation
  const handleKeyDown = (e: React.KeyboardEvent<HTMLButtonElement>) => {
    const currentIdx = btnRef.current ? triggerNodes.indexOf(btnRef.current) : -1
    switch (e.key) {
      case 'ArrowRight':
      case 'ArrowDown':
        e.preventDefault()
        if (currentIdx !== -1 && focusNext) focusNext(currentIdx)
        break
      case 'ArrowLeft':
      case 'ArrowUp':
        e.preventDefault()
        if (currentIdx !== -1 && focusPrev) focusPrev(currentIdx)
        break
      case 'Home':
        e.preventDefault()
        if (focusFirst) focusFirst()
        break
      case 'End':
        e.preventDefault()
        if (focusLast) focusLast()
        break
      case 'Enter':
      case ' ':
        e.preventDefault()
        stepper.goTo(step.id)
        break
    }
  }

  if (asChild) {
    return (
      <span data-slot="stepper-trigger" data-state={state} className={className}>
        {children}
      </span>
    )
  }

  return (
    <button
      ref={triggerRef}
      role="tab"
      id={id}
      aria-selected={isSelected}
      aria-controls={panelId}
      tabIndex={typeof tabIndex === 'number' ? tabIndex : isSelected ? 0 : -1}
      data-slot="stepper-trigger"
      data-state={state}
      data-loading={isLoading}
      className={cn(
        'inline-flex cursor-pointer items-center outline-none disabled:pointer-events-none disabled:opacity-60',
        'gap-2.5 rounded-full',
        className
      )}
      onClick={() => stepper.goTo(step.id)}
      onKeyDown={handleKeyDown}
      disabled={isDisabled}
      {...props}
    >
      {children}
    </button>
  )
}

interface StepperIndicatorProps extends React.ComponentProps<'div'> {
  variant?: 'default' | 'outline'
}

function StepperIndicator({ children, className, variant = 'default' }: StepperIndicatorProps) {
  const { state, isLoading, step } = useStepItem()
  const { indicators } = useStepper()

  const base =
    'relative flex size-8 shrink-0 items-center justify-center overflow-hidden transition-all duration-300 rounded-md text-sm font-medium'

  const defaultClasses = cn(
    'border-background bg-muted data-[state=active]:bg-primary data-[state=active]:text-primary-foreground data-[state=completed]:bg-primary data-[state=completed]:text-primary-foreground',
    base
  )

  const outlineClasses = cn(
    'border border-primary/20 bg-transparent text-muted-foreground data-[state=active]:border-primary data-[state=active]:text-foreground data-[state=completed]:border-foreground data-[state=completed]:text-foreground',
    base
  )

  const classes = variant === 'outline' ? outlineClasses : defaultClasses

  return (
    <div data-slot="stepper-indicator" data-state={state} className={cn(classes, className)}>
      <div className="absolute">
        {(isLoading ? indicators?.loading : indicators?.[state]) ??
          (step?.icon ? <span className="*:[svg]:size-4">{step.icon}</span> : children)}
      </div>
    </div>
  )
}

function StepperSeparator({ className }: React.ComponentProps<'div'>) {
  const { state } = useStepItem()

  return (
    <div
      data-slot="stepper-separator"
      data-state={state}
      className={cn(
        'm-2 rounded-sm bg-muted transition-colors duration-500 group-data-[orientation=horizontal]/stepper-nav:h-0.5 group-data-[orientation=horizontal]/stepper-nav:flex-1 group-data-[orientation=vertical]/stepper-nav:h-12 group-data-[orientation=vertical]/stepper-nav:w-0.5 group-data-[state=completed]/step:bg-primary',
        className
      )}
    />
  )
}

function StepperTitle({ children, className }: React.ComponentProps<'h3'>) {
  const { state } = useStepItem()

  return (
    <h3
      data-slot="stepper-title"
      data-state={state}
      className={cn('text-sm font-medium', className)}
    >
      {children}
    </h3>
  )
}

function StepperDescription({ children, className }: React.ComponentProps<'div'>) {
  const { state } = useStepItem()

  return (
    <div
      data-slot="stepper-description"
      data-state={state}
      className={cn('text-xs font-medium text-muted-foreground', className)}
    >
      {children}
    </div>
  )
}

function StepperNav({ children, className }: React.ComponentProps<'nav'>) {
  const { stepper, orientation, configOrientation, responsive } = useStepper()

  const responsiveNavClasses =
    responsive && configOrientation === 'horizontal' ? 'flex-col md:flex-row md:w-full' : ''

  return (
    <nav
      data-slot="stepper-nav"
      data-state={stepper.id}
      data-orientation={orientation}
      className={cn(
        'group/stepper-nav inline-flex data-[orientation=horizontal]:w-full data-[orientation=horizontal]:flex-row data-[orientation=vertical]:flex-col',
        responsiveNavClasses,
        className
      )}
    >
      {children}
    </nav>
  )
}

function StepperPanel({ children, className }: React.ComponentProps<'div'>) {
  const { stepper } = useStepper()

  return (
    <div data-slot="stepper-panel" data-state={stepper.id} className={cn('w-full', className)}>
      {children}
    </div>
  )
}

interface StepperContentProps extends React.ComponentProps<'div'> {
  value: string
  forceMount?: boolean
}

function StepperContent({ value, forceMount, children, className }: StepperContentProps) {
  const { stepper } = useStepper()
  const isActive = value === stepper.id

  if (!forceMount && !isActive) {
    return null
  }

  return (
    <div
      role="tabpanel"
      id={`stepper-panel-${value}`}
      aria-labelledby={`stepper-tab-${value}`}
      data-slot="stepper-content"
      data-state={stepper.id}
      className={cn('w-full', className, !isActive && forceMount && 'hidden')}
      hidden={!isActive && forceMount}
    >
      {children}
    </div>
  )
}

export {
  useStepper,
  useStepItem,
  Stepper,
  StepperItem,
  StepperTrigger,
  StepperIndicator,
  StepperSeparator,
  StepperTitle,
  StepperDescription,
  StepperPanel,
  StepperContent,
  StepperNav,
  type StepperProps,
  type StepperItemProps,
  type StepperTriggerProps,
  type StepperContentProps
}
