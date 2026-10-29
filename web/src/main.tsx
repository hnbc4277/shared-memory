import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import App from "./App";
import "./styles/pearl-shared.css";
import "./styles/pearl-pages.css";
import "./styles/sm-bridge.css";
import "./styles.css";
import "./styles/flat-rect.css";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
