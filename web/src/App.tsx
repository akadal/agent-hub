import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom'

import { AppShell } from '@/components/layout/app-shell'
import { AuthProvider } from '@/lib/auth'
import { DashboardPage } from '@/pages/dashboard'
import { LoginPage } from '@/pages/login'
import { MachinesPage } from '@/pages/machines'
import { PlaceholderPage } from '@/pages/placeholder'
import { WorkspacePage } from '@/pages/workspace'

export default function App() {
  return (
    <AuthProvider>
      <BrowserRouter>
        <Routes>
          <Route path="/login" element={<LoginPage />} />
          <Route element={<AppShell />}>
            <Route index element={<DashboardPage />} />
            <Route path="workspace" element={<WorkspacePage />} />
            <Route path="workspace/:machineId" element={<WorkspacePage />} />
            <Route path="machines" element={<MachinesPage />} />
            <Route path="terminals" element={<Navigate to="/workspace" replace />} />
            <Route
              path="terminals/:id"
              element={<Navigate to="/workspace" replace />}
            />
            <Route
              path="users"
              element={
                <PlaceholderPage
                  title="Users"
                  description="Multi-user management comes in a later milestone. Bootstrap admin is active."
                />
              }
            />
            <Route
              path="permissions"
              element={
                <PlaceholderPage
                  title="Permissions"
                  description="Admin can access all machines for this release."
                />
              }
            />
            <Route
              path="audit"
              element={
                <PlaceholderPage
                  title="Audit"
                  description="Audit log UI is planned for a later release."
                />
              }
            />
            <Route
              path="settings"
              element={
                <PlaceholderPage
                  title="Settings"
                  description="Access policy UI later. Local use does not require Tailscale."
                />
              }
            />
            <Route path="*" element={<Navigate to="/" replace />} />
          </Route>
        </Routes>
      </BrowserRouter>
    </AuthProvider>
  )
}
