import type {
  FuzzyFileSearchMatchType,
  FuzzyFileSearchParams,
  FuzzyFileSearchResult,
  FuzzyFileSearchResponse,
  FuzzyFileSearchSessionCompletedNotification,
  FuzzyFileSearchSessionUpdatedNotification,
  MethodParamsByName,
  MethodResultsByName,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
  (<T>() => T extends B ? 1 : 2)
    ? true
    : false;
type Expect<T extends true> = T;
type IsUnknown<T> = unknown extends T
  ? ([keyof T] extends [never] ? true : false)
  : false;
type ParamsForKnownMethod<M extends string> = M extends keyof MethodParamsByName
  ? MethodParamsByName[M]
  : unknown;
type ResultForKnownMethod<M extends string> = M extends keyof MethodResultsByName
  ? MethodResultsByName[M]
  : unknown;

type Contracts = [
  Expect<Equal<FuzzyFileSearchMatchType, "file" | "directory">>,
  Expect<Equal<
    FuzzyFileSearchParams,
    { query: string; roots: Array<string>; cancellationToken: string | null }
  >>,
  Expect<Equal<
    FuzzyFileSearchResult,
    {
      root: string;
      path: string;
      match_type: FuzzyFileSearchMatchType;
      file_name: string;
      score: number;
      indices: Array<number> | null;
    }
  >>,
  Expect<Equal<FuzzyFileSearchResponse, { files: Array<FuzzyFileSearchResult> }>>,
  Expect<Equal<
    FuzzyFileSearchSessionUpdatedNotification,
    { sessionId: string; query: string; files: Array<FuzzyFileSearchResult> }
  >>,
  Expect<Equal<FuzzyFileSearchSessionCompletedNotification, { sessionId: string }>>,
  Expect<IsUnknown<ParamsForKnownMethod<"fuzzyFileSearch">>>,
  Expect<IsUnknown<ResultForKnownMethod<"fuzzyFileSearch">>>,
  Expect<IsUnknown<ParamsForKnownMethod<"fuzzyFileSearch/sessionUpdated">>>,
  Expect<IsUnknown<ResultForKnownMethod<"fuzzyFileSearch/sessionUpdated">>>,
  Expect<IsUnknown<ParamsForKnownMethod<"fuzzyFileSearch/sessionCompleted">>>,
  Expect<IsUnknown<ResultForKnownMethod<"fuzzyFileSearch/sessionCompleted">>>,
];

declare const contracts: Contracts;
void contracts;

const file = "file" satisfies FuzzyFileSearchMatchType;
const directory = "directory" satisfies FuzzyFileSearchMatchType;
const params = {
  query: "",
  roots: ["", "repo/../repo", "", "repo/../repo"],
  cancellationToken: null,
} satisfies FuzzyFileSearchParams;
const result = {
  root: "",
  path: "",
  match_type: file,
  file_name: "",
  score: 0,
  indices: null,
} satisfies FuzzyFileSearchResult;
({
  ...result,
  root: " repo ",
  path: "./a/../b",
  match_type: directory,
  file_name: " name ",
  score: 4_294_967_295,
  indices: [4_294_967_295, 0, 4_294_967_295],
}) satisfies FuzzyFileSearchResult;
({ files: [] }) satisfies FuzzyFileSearchResponse;
({ files: [result, result] }) satisfies FuzzyFileSearchResponse;
({ sessionId: "", query: "", files: [] }) satisfies FuzzyFileSearchSessionUpdatedNotification;
({ sessionId: " session ", query: " query ", files: [result, result] }) satisfies FuzzyFileSearchSessionUpdatedNotification;
({ sessionId: "" }) satisfies FuzzyFileSearchSessionCompletedNotification;

// @ts-expect-error match types are closed.
"folder" satisfies FuzzyFileSearchMatchType;
// @ts-expect-error query is required.
({ roots: [], cancellationToken: null }) satisfies FuzzyFileSearchParams;
// @ts-expect-error roots are required.
({ query: "", cancellationToken: null }) satisfies FuzzyFileSearchParams;
// @ts-expect-error cancellationToken is required by exact ts-rs output.
({ query: "", roots: [] }) satisfies FuzzyFileSearchParams;
// @ts-expect-error roots are non-null.
({ ...params, roots: null }) satisfies FuzzyFileSearchParams;
// @ts-expect-error cancellationToken is nullable string only.
({ ...params, cancellationToken: 1 }) satisfies FuzzyFileSearchParams;
// @ts-expect-error snake-case aliases do not replace canonical params fields.
({ query: "", roots: [], cancellation_token: null }) satisfies FuzzyFileSearchParams;
// @ts-expect-error fields absent from params are rejected.
({ ...params, extra: true }) satisfies FuzzyFileSearchParams;

// @ts-expect-error root is required.
({ path: "", match_type: file, file_name: "", score: 0, indices: null }) satisfies FuzzyFileSearchResult;
// @ts-expect-error path is required.
({ root: "", match_type: file, file_name: "", score: 0, indices: null }) satisfies FuzzyFileSearchResult;
// @ts-expect-error match_type is required.
({ root: "", path: "", file_name: "", score: 0, indices: null }) satisfies FuzzyFileSearchResult;
// @ts-expect-error file_name is required.
({ root: "", path: "", match_type: file, score: 0, indices: null }) satisfies FuzzyFileSearchResult;
// @ts-expect-error score is required.
({ root: "", path: "", match_type: file, file_name: "", indices: null }) satisfies FuzzyFileSearchResult;
// @ts-expect-error indices is required by exact ts-rs output.
({ root: "", path: "", match_type: file, file_name: "", score: 0 }) satisfies FuzzyFileSearchResult;
// @ts-expect-error result strings are non-null.
({ ...result, root: null }) satisfies FuzzyFileSearchResult;
// @ts-expect-error score is numeric.
({ ...result, score: "0" }) satisfies FuzzyFileSearchResult;
// @ts-expect-error indices are an array or null.
({ ...result, indices: 0 }) satisfies FuzzyFileSearchResult;
// @ts-expect-error result names intentionally remain snake case.
({ ...result, match_type: undefined, matchType: file }) satisfies FuzzyFileSearchResult;
// @ts-expect-error result names intentionally remain snake case.
({ ...result, file_name: undefined, fileName: "" }) satisfies FuzzyFileSearchResult;
// @ts-expect-error fields absent from results are rejected.
({ ...result, extra: true }) satisfies FuzzyFileSearchResult;

// @ts-expect-error files are required.
({}) satisfies FuzzyFileSearchResponse;
// @ts-expect-error files are non-null.
({ files: null }) satisfies FuzzyFileSearchResponse;
// @ts-expect-error response entries retain exact result fields.
({ files: [{ root: "", path: "", match_type: file, file_name: "", score: 0 }] }) satisfies FuzzyFileSearchResponse;
// @ts-expect-error fields absent from responses are rejected.
({ files: [], extra: true }) satisfies FuzzyFileSearchResponse;

// @ts-expect-error sessionId is required.
({ query: "", files: [] }) satisfies FuzzyFileSearchSessionUpdatedNotification;
// @ts-expect-error query is required.
({ sessionId: "", files: [] }) satisfies FuzzyFileSearchSessionUpdatedNotification;
// @ts-expect-error files are required.
({ sessionId: "", query: "" }) satisfies FuzzyFileSearchSessionUpdatedNotification;
// @ts-expect-error files are non-null.
({ sessionId: "", query: "", files: null }) satisfies FuzzyFileSearchSessionUpdatedNotification;
// @ts-expect-error snake-case aliases do not replace canonical notification fields.
({ session_id: "", query: "", files: [] }) satisfies FuzzyFileSearchSessionUpdatedNotification;
// @ts-expect-error fields absent from updated notifications are rejected.
({ sessionId: "", query: "", files: [], extra: true }) satisfies FuzzyFileSearchSessionUpdatedNotification;

// @ts-expect-error sessionId is required.
({}) satisfies FuzzyFileSearchSessionCompletedNotification;
// @ts-expect-error sessionId is non-null.
({ sessionId: null }) satisfies FuzzyFileSearchSessionCompletedNotification;
// @ts-expect-error snake-case aliases do not replace canonical notification fields.
({ session_id: "" }) satisfies FuzzyFileSearchSessionCompletedNotification;
// @ts-expect-error fields absent from completed notifications are rejected.
({ sessionId: "", extra: true }) satisfies FuzzyFileSearchSessionCompletedNotification;
