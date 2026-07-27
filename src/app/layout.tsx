import type { Metadata } from "next";
import type { ReactNode } from "react";

export const metadata: Metadata = {
  title: "zachmanson.com API",
  description: "General-purpose API backing a few zachmanson.com projects.",
};

export default function RootLayout({ children }: { children: ReactNode }) {
  return (
    <html lang="en">
      <body
        style={{
          margin: 0,
          fontFamily:
            "-apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif",
          background: "#0f1115",
          color: "#e6e9ef",
        }}
      >
        {children}
      </body>
    </html>
  );
}
