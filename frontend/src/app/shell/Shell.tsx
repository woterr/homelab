import { Outlet } from 'react-router-dom'
import { CommandBar } from './CommandBar'
import { ToastStack } from '../../components/ui/ToastStack'

export function Shell() {
  return (
    <>
      <div className="workspace">
        <CommandBar />
        <main className="workspace__main">
          <Outlet />
        </main>
      </div>
      <ToastStack />
    </>
  )
}