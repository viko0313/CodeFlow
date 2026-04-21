import NextAuth from "next-auth";
import Credentials from "next-auth/providers/credentials";

export const { handlers, signIn, signOut, auth } = NextAuth({
  trustHost: true,
  session: { strategy: "jwt" },
  pages: {
    signIn: "/login",
  },
  providers: [
    Credentials({
      name: "CodeFlow local login",
      credentials: {
        username: { label: "Username", type: "text" },
        password: { label: "Password", type: "password" },
      },
      authorize(credentials) {
        const expectedUser = process.env.CODEFLOW_WEB_USER ?? "admin";
        const expectedPassword = process.env.CODEFLOW_WEB_PASSWORD ?? "codeflow";
        if (
          credentials?.username === expectedUser &&
          credentials?.password === expectedPassword
        ) {
          return {
            id: "local-dev",
            name: expectedUser,
            email: `${expectedUser}@codeflow.local`,
          };
        }
        return null;
      },
    }),
  ],
});
