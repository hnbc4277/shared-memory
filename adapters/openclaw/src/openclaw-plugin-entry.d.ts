/** Minimal stub so this package typechecks without openclaw installed. */
declare module "openclaw/plugin-sdk/plugin-entry" {
  export type OpenClawPluginApi = {
    on: (
      hook: string,
      handler: (event: any, ctx: any) => any,
      opts?: Record<string, unknown>,
    ) => void;
  };

  export type DefinePluginEntryOptions = {
    id: string;
    name: string;
    description: string;
    register: (api: OpenClawPluginApi) => void;
  };

  export function definePluginEntry(
    options: DefinePluginEntryOptions,
  ): DefinePluginEntryOptions;
}
