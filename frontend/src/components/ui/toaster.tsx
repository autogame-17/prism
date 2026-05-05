import { Toaster as SonnerToaster, toast } from 'sonner'

export function Toaster() {
  return (
    <SonnerToaster
      theme="dark"
      position="bottom-right"
      richColors
      closeButton
      offset={16}
      toastOptions={{
        className: 'font-sans',
        classNames: {
          toast:
            'max-w-[min(calc(100vw-2rem),420px)] break-words',
          description: 'break-all',
        },
      }}
    />
  )
}

export { toast }
