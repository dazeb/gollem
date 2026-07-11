import type {
  ResultFor,
  ToolRequestUserInputAnswer,
  ToolRequestUserInputOption,
  ToolRequestUserInputParams,
  ToolRequestUserInputQuestion,
  ToolRequestUserInputResponse,
} from "../gollem_appserver_protocol";

export const option = { label: "safe", description: "" } satisfies ToolRequestUserInputOption;
export const question = {
  id: "call-1",
  header: "Mode",
  question: "Choose a mode",
  isOther: false,
  isSecret: false,
  options: [option],
} satisfies ToolRequestUserInputQuestion;
export const params = {
  threadId: "thread-1",
  turnId: "turn-1",
  itemId: "item-1",
  questions: [question],
  autoResolutionMs: null,
  prompt: "Choose a mode",
} satisfies ToolRequestUserInputParams;
export const answer = { answers: ["safe"] } satisfies ToolRequestUserInputAnswer;
export const response = { answers: { "call-1": answer } } satisfies ToolRequestUserInputResponse;
response satisfies ResultFor<"item/tool/requestUserInput">;

// @ts-expect-error autoResolutionMs is required and explicitly nullable.
export const rejectMissingResolution = { ...params, autoResolutionMs: undefined } satisfies ToolRequestUserInputParams;
// @ts-expect-error question options are required and explicitly nullable.
export const rejectMissingOptions = { ...question, options: undefined } satisfies ToolRequestUserInputQuestion;
// @ts-expect-error option descriptions are required.
export const rejectMissingDescription = { label: "safe" } satisfies ToolRequestUserInputOption;
// @ts-expect-error answer arrays are required.
export const rejectMissingAnswers = {} satisfies ToolRequestUserInputAnswer;
// @ts-expect-error response answer maps are required.
export const rejectMissingAnswerMap = {} satisfies ToolRequestUserInputResponse;
