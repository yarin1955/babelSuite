export interface User {
  userId: string
  workspaceId: string
  username: string
  email: string
  fullName: string
  isAdmin: boolean
  createdAt: string
}

export interface Workspace {
  workspaceId: string
  slug: string
  name: string
  createdAt: string
}

export interface AuthResponse {
  token: string
  user: User
  workspace: Workspace
  expiresAt: string
}
