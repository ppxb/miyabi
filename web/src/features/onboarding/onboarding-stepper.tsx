import {
  Stepper,
  StepperIndicator,
  StepperItem,
  StepperNav,
  StepperSeparator,
  StepperTitle,
  StepperTrigger
} from '@/components/ui/stepper'

export type OnboardingStep = {
  id: string
  title: string
}

export function OnboardingStepper({
  steps,
  current,
  onChange
}: {
  steps: OnboardingStep[]
  current: string
  onChange: (id: string) => void
}) {
  const currentIndex = Math.max(
    0,
    steps.findIndex(step => step.id === current)
  )

  return (
    <Stepper
      steps={steps}
      value={current}
      onValueChange={onChange}
      orientation="horizontal"
      className="w-full"
    >
      <StepperNav className="gap-1 overflow-x-auto py-1">
        {steps.map((step, index) => (
          <StepperItem key={step.id} stepId={step.id} disabled={index > currentIndex}>
            <StepperTrigger className="gap-2 rounded-full px-1 py-1 outline-none focus:outline-none focus-visible:outline-none">
              <StepperIndicator className="size-6 text-xs">{index + 1}</StepperIndicator>
              <StepperTitle className="text-xs whitespace-nowrap">{step.title}</StepperTitle>
            </StepperTrigger>
            {index < steps.length - 1 ? <StepperSeparator className="mx-1 h-0.5 flex-1" /> : null}
          </StepperItem>
        ))}
      </StepperNav>
    </Stepper>
  )
}
