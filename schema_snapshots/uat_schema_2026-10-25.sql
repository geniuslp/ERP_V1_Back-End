--
-- PostgreSQL database dump
--

\restrict OZc71CZbkAt997clByLeIPzKsV8VgtLPxMIfF6jgYtEAwxLgaiwd1mmJ8S5SmOk

-- Dumped from database version 16.14 (Ubuntu 16.14-0ubuntu0.24.04.1)
-- Dumped by pg_dump version 18.4

-- Started on 2026-10-25 15:06:10

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET transaction_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

--
-- TOC entry 5166 (class 0 OID 0)
-- Dependencies: 4
-- Name: SCHEMA public; Type: COMMENT; Schema: -; Owner: pg_database_owner
--

COMMENT ON SCHEMA public IS '';


--
-- TOC entry 436 (class 1255 OID 25821)
-- Name: chk_project_job_share_total(); Type: FUNCTION; Schema: public; Owner: postgres
--

CREATE FUNCTION public.chk_project_job_share_total() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE v_total numeric;
BEGIN
  SELECT COALESCE(SUM(share_percent), 0) INTO v_total
  FROM public.project_job_share WHERE project_id = NEW.project_id;
  IF v_total > 100 THEN
    RAISE EXCEPTION 'project_job_share total % exceeds 100 for project %', v_total, NEW.project_id;
  END IF;
  RETURN NULL;
END;
$$;


ALTER FUNCTION public.chk_project_job_share_total() OWNER TO postgres;

--
-- TOC entry 435 (class 1255 OID 20762)
-- Name: enforce_read_before_write(); Type: FUNCTION; Schema: public; Owner: postgres
--

CREATE FUNCTION public.enforce_read_before_write() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  IF NEW.can_read = false THEN
    NEW.can_write  := false;
    NEW.can_update := false;
    NEW.can_delete := false;
  END IF;
  RETURN NEW;
END;
$$;


ALTER FUNCTION public.enforce_read_before_write() OWNER TO postgres;

--
-- TOC entry 437 (class 1255 OID 25872)
-- Name: trg_project_cost_budget_item_only(); Type: FUNCTION; Schema: public; Owner: postgres
--

CREATE FUNCTION public.trg_project_cost_budget_item_only() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    v_head  boolean;
    v_major boolean;
BEGIN
    SELECT is_head, is_major INTO v_head, v_major
    FROM public.cost_subgroup WHERE id = NEW.cost_subgroup_id;

    IF v_head OR v_major THEN
        RAISE EXCEPTION 'งบประมาณกำหนดได้เฉพาะรหัสต้นทุนระดับรายการเท่านั้น (ไม่ใช่รหัสระดับ Head หรือ Major)';
    END IF;
    RETURN NEW;
END;
$$;


ALTER FUNCTION public.trg_project_cost_budget_item_only() OWNER TO postgres;

SET default_tablespace = '';

SET default_table_access_method = heap;

--
-- TOC entry 217 (class 1259 OID 20763)
-- Name: approval_config; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.approval_config (
    id bigint NOT NULL,
    doc_type character varying(30) NOT NULL,
    step_no integer NOT NULL,
    step_name character varying(200) NOT NULL,
    approver_role_id bigint,
    min_amount numeric(18,4),
    max_amount numeric(18,4),
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint
);


ALTER TABLE public.approval_config OWNER TO postgres;

--
-- TOC entry 218 (class 1259 OID 20769)
-- Name: approval_config_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.approval_config ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.approval_config_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- TOC entry 219 (class 1259 OID 20770)
-- Name: approval_delegation; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.approval_delegation (
    id bigint NOT NULL,
    doc_type character varying(30),
    user_id bigint NOT NULL,
    reason text,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint
);


ALTER TABLE public.approval_delegation OWNER TO postgres;

--
-- TOC entry 220 (class 1259 OID 20776)
-- Name: approval_delegation_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.approval_delegation ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.approval_delegation_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- TOC entry 221 (class 1259 OID 20777)
-- Name: approval_doc_types; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.approval_doc_types (
    id bigint NOT NULL,
    doc_type character varying(30) NOT NULL,
    doc_label character varying(200) NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint,
    table_name character varying(50),
    id_column character varying(50) DEFAULT 'id'::character varying,
    status_column character varying(50) DEFAULT 'status'::character varying,
    approved_value character varying(30) DEFAULT 'APPROVED'::character varying,
    rejected_value character varying(30) DEFAULT 'REJECTED'::character varying,
    status_log_table character varying(50),
    status_log_fk_column character varying(50)
);


ALTER TABLE public.approval_doc_types OWNER TO postgres;

--
-- TOC entry 222 (class 1259 OID 20789)
-- Name: approval_doc_types_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.approval_doc_types ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.approval_doc_types_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- TOC entry 223 (class 1259 OID 20790)
-- Name: approval_log; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.approval_log (
    id bigint NOT NULL,
    approval_id bigint NOT NULL,
    doc_type character varying(30) NOT NULL,
    doc_id bigint NOT NULL,
    doc_no character varying(30) NOT NULL,
    step_no integer NOT NULL,
    action character varying(20) NOT NULL,
    action_by bigint NOT NULL,
    action_at timestamp without time zone DEFAULT now() NOT NULL,
    comments text,
    old_status character varying(20),
    new_status character varying(20),
    ip_address inet,
    user_agent text,
    CONSTRAINT approval_log_action_check CHECK (((action)::text = ANY (ARRAY['SUBMIT'::text, 'APPROVE'::text, 'REJECT'::text, 'RETURN'::text, 'CANCEL'::text, 'ESCALATE'::text, 'COMMENT'::text])))
);


ALTER TABLE public.approval_log OWNER TO postgres;

--
-- TOC entry 224 (class 1259 OID 20797)
-- Name: approval_log_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.approval_log ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.approval_log_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- TOC entry 225 (class 1259 OID 20798)
-- Name: approval_request; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.approval_request (
    id bigint NOT NULL,
    doc_type character varying(30) NOT NULL,
    doc_id bigint NOT NULL,
    doc_no character varying(30) NOT NULL,
    step_no integer NOT NULL,
    requested_by bigint NOT NULL,
    assigned_to bigint,
    status character varying(20) DEFAULT 'PENDING'::character varying NOT NULL,
    due_date date,
    amount numeric(18,4),
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint,
    CONSTRAINT approval_request_status_check CHECK (((status)::text = ANY (ARRAY['PENDING'::text, 'APPROVED'::text, 'REJECTED'::text, 'CANCELLED'::text, 'ESCALATED'::text])))
);


ALTER TABLE public.approval_request OWNER TO postgres;

--
-- TOC entry 226 (class 1259 OID 20805)
-- Name: approval_request_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.approval_request ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.approval_request_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- TOC entry 227 (class 1259 OID 20806)
-- Name: borrow; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.borrow (
    id bigint NOT NULL,
    borrow_no character varying(30) NOT NULL,
    borrow_type character varying(20) DEFAULT 'BORROW'::character varying NOT NULL,
    warehouse_code character varying(20) NOT NULL,
    borrower_id bigint NOT NULL,
    location_code character varying(20),
    borrow_date date DEFAULT CURRENT_DATE NOT NULL,
    expected_return date,
    actual_return date,
    status character varying(20) DEFAULT 'OPEN'::character varying NOT NULL,
    remarks text,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint NOT NULL,
    updated_by bigint,
    purpose text,
    approved_by bigint,
    approved_at timestamp without time zone,
    project_code character varying(50),
    CONSTRAINT borrow_status_check CHECK (((status)::text = ANY (ARRAY['DRAFT'::text, 'PENDING_APPROVAL'::text, 'APPROVED'::text, 'REJECTED'::text, 'BORROWED'::text, 'RETURNED'::text, 'PARTIALLY_RETURNED'::text, 'CANCELLED'::text]))),
    CONSTRAINT borrow_type_check CHECK (((borrow_type)::text = ANY (ARRAY['BORROW'::text, 'RETURN'::text])))
);


ALTER TABLE public.borrow OWNER TO postgres;

--
-- TOC entry 5167 (class 0 OID 0)
-- Dependencies: 227
-- Name: COLUMN borrow.purpose; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.borrow.purpose IS 'วัตถุประสงค์การยืม';


--
-- TOC entry 5168 (class 0 OID 0)
-- Dependencies: 227
-- Name: COLUMN borrow.approved_by; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.borrow.approved_by IS 'ผู้อนุมัติ';


--
-- TOC entry 228 (class 1259 OID 20818)
-- Name: borrow_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.borrow ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.borrow_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- TOC entry 229 (class 1259 OID 20819)
-- Name: borrow_line; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.borrow_line (
    id bigint NOT NULL,
    borrow_id bigint NOT NULL,
    mat_code character varying(20) NOT NULL,
    mat_type character varying(20) DEFAULT 'RETURNABLE'::character varying NOT NULL,
    qty_borrowed numeric(18,4) NOT NULL,
    qty_returned numeric(18,4) DEFAULT 0 NOT NULL,
    condition_out character varying(50),
    condition_in character varying(50),
    remarks text,
    stock_item_id bigint,
    location_code character varying(20),
    qty_requested numeric(18,4),
    qty_approved numeric(18,4),
    line_no integer,
    unit character varying(50),
    CONSTRAINT borrow_line_condition_in_check CHECK (((condition_in)::text = ANY (ARRAY['GOOD'::text, 'FAIR'::text, 'DAMAGED'::text]))),
    CONSTRAINT borrow_line_condition_out_check CHECK (((condition_out)::text = ANY (ARRAY['GOOD'::text, 'FAIR'::text, 'DAMAGED'::text]))),
    CONSTRAINT borrow_line_mat_type_check CHECK (((mat_type)::text = ANY (ARRAY['RETURNABLE'::text, 'CONSUMABLE'::text]))),
    CONSTRAINT borrow_line_qty_borrowed_check CHECK ((qty_borrowed > (0)::numeric))
);


ALTER TABLE public.borrow_line OWNER TO postgres;

--
-- TOC entry 5169 (class 0 OID 0)
-- Dependencies: 229
-- Name: COLUMN borrow_line.stock_item_id; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.borrow_line.stock_item_id IS 'FK → stock_item (ใหม่) แทน mat_code';


--
-- TOC entry 5170 (class 0 OID 0)
-- Dependencies: 229
-- Name: COLUMN borrow_line.qty_requested; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.borrow_line.qty_requested IS 'จำนวนที่ขอยืม';


--
-- TOC entry 5171 (class 0 OID 0)
-- Dependencies: 229
-- Name: COLUMN borrow_line.qty_approved; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.borrow_line.qty_approved IS 'จำนวนที่อนุมัติ (อาจน้อยกว่าที่ขอ)';


--
-- TOC entry 230 (class 1259 OID 20830)
-- Name: borrow_line_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.borrow_line ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.borrow_line_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- TOC entry 231 (class 1259 OID 20831)
-- Name: borrow_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.borrow_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.borrow_seq OWNER TO postgres;

--
-- TOC entry 232 (class 1259 OID 20832)
-- Name: borrow_status_log; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.borrow_status_log (
    id bigint NOT NULL,
    borrow_id bigint NOT NULL,
    from_status character varying(30),
    to_status character varying(30) NOT NULL,
    changed_by bigint NOT NULL,
    changed_at timestamp without time zone DEFAULT now() NOT NULL,
    remarks text
);


ALTER TABLE public.borrow_status_log OWNER TO postgres;

--
-- TOC entry 5172 (class 0 OID 0)
-- Dependencies: 232
-- Name: TABLE borrow_status_log; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON TABLE public.borrow_status_log IS 'ประวัติการเปลี่ยนสถานะการยืม';


--
-- TOC entry 233 (class 1259 OID 20838)
-- Name: borrow_status_log_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.borrow_status_log ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.borrow_status_log_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- TOC entry 234 (class 1259 OID 20839)
-- Name: brand; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.brand (
    id integer NOT NULL,
    brand_code character varying(10) NOT NULL,
    spec_id integer NOT NULL,
    brand_name character varying(100) NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint
);


ALTER TABLE public.brand OWNER TO postgres;

--
-- TOC entry 235 (class 1259 OID 20845)
-- Name: brand_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.brand_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.brand_id_seq OWNER TO postgres;

--
-- TOC entry 5173 (class 0 OID 0)
-- Dependencies: 235
-- Name: brand_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.brand_id_seq OWNED BY public.brand.id;


--
-- TOC entry 236 (class 1259 OID 20846)
-- Name: cost_group; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.cost_group (
    id integer NOT NULL,
    job_id integer NOT NULL,
    group_code character varying(5) NOT NULL,
    group_name character varying(100) NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint
);


ALTER TABLE public.cost_group OWNER TO postgres;

--
-- TOC entry 237 (class 1259 OID 20852)
-- Name: cost_group_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.cost_group_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.cost_group_id_seq OWNER TO postgres;

--
-- TOC entry 5174 (class 0 OID 0)
-- Dependencies: 237
-- Name: cost_group_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.cost_group_id_seq OWNED BY public.cost_group.id;


--
-- TOC entry 238 (class 1259 OID 20853)
-- Name: cost_job; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.cost_job (
    id integer NOT NULL,
    subject_id integer NOT NULL,
    job_code character varying(5) NOT NULL,
    job_name character varying(100) NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint
);


ALTER TABLE public.cost_job OWNER TO postgres;

--
-- TOC entry 239 (class 1259 OID 20859)
-- Name: cost_job_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.cost_job_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.cost_job_id_seq OWNER TO postgres;

--
-- TOC entry 5175 (class 0 OID 0)
-- Dependencies: 239
-- Name: cost_job_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.cost_job_id_seq OWNED BY public.cost_job.id;


--
-- TOC entry 240 (class 1259 OID 20860)
-- Name: cost_subgroup; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.cost_subgroup (
    id integer NOT NULL,
    group_id integer NOT NULL,
    subgroup_code character varying(5) NOT NULL,
    subgroup_name character varying(100),
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint,
    is_head boolean DEFAULT false NOT NULL,
    is_major boolean DEFAULT false NOT NULL,
    CONSTRAINT chk_cost_subgroup_major_not_head CHECK ((NOT (is_major AND is_head)))
);


ALTER TABLE public.cost_subgroup OWNER TO postgres;

--
-- TOC entry 241 (class 1259 OID 20866)
-- Name: cost_subgroup_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.cost_subgroup_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.cost_subgroup_id_seq OWNER TO postgres;

--
-- TOC entry 5176 (class 0 OID 0)
-- Dependencies: 241
-- Name: cost_subgroup_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.cost_subgroup_id_seq OWNED BY public.cost_subgroup.id;


--
-- TOC entry 242 (class 1259 OID 20867)
-- Name: cost_subject; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.cost_subject (
    id integer NOT NULL,
    subject_code character varying(5) NOT NULL,
    subject_name character varying(100) NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint
);


ALTER TABLE public.cost_subject OWNER TO postgres;

--
-- TOC entry 243 (class 1259 OID 20873)
-- Name: cost_subject_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.cost_subject_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.cost_subject_id_seq OWNER TO postgres;

--
-- TOC entry 5177 (class 0 OID 0)
-- Dependencies: 243
-- Name: cost_subject_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.cost_subject_id_seq OWNED BY public.cost_subject.id;


--
-- TOC entry 244 (class 1259 OID 20874)
-- Name: customer; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.customer (
    cus_id integer NOT NULL,
    customer_code character varying(20) NOT NULL,
    customer_name character varying(255) NOT NULL,
    address text,
    contact text,
    credit_term character varying(50),
    remarks text,
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    tel character varying(50),
    updated_by bigint,
    contact_phone character varying(50),
    tax_id character varying(20)
);


ALTER TABLE public.customer OWNER TO postgres;

--
-- TOC entry 245 (class 1259 OID 20882)
-- Name: customer_cus_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.customer_cus_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.customer_cus_id_seq OWNER TO postgres;

--
-- TOC entry 5178 (class 0 OID 0)
-- Dependencies: 245
-- Name: customer_cus_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.customer_cus_id_seq OWNED BY public.customer.cus_id;


--
-- TOC entry 246 (class 1259 OID 20883)
-- Name: departments; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.departments (
    id bigint NOT NULL,
    dept_code character varying(50) NOT NULL,
    dept_name character varying(200) NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    sort_order integer DEFAULT 0 NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    short_code character varying(10)
);


ALTER TABLE public.departments OWNER TO postgres;

--
-- TOC entry 247 (class 1259 OID 20890)
-- Name: departments_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.departments ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.departments_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- TOC entry 248 (class 1259 OID 20891)
-- Name: dept_menu_permissions; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.dept_menu_permissions (
    id bigint NOT NULL,
    menu_id bigint NOT NULL,
    can_read boolean DEFAULT false NOT NULL,
    can_write boolean DEFAULT false NOT NULL,
    can_update boolean DEFAULT false NOT NULL,
    can_delete boolean DEFAULT false NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint,
    dept_code character varying(50) NOT NULL
);


ALTER TABLE public.dept_menu_permissions OWNER TO postgres;

--
-- TOC entry 249 (class 1259 OID 20900)
-- Name: dept_menu_permissions_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.dept_menu_permissions ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.dept_menu_permissions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- TOC entry 421 (class 1259 OID 23735)
-- Name: doc_number_counter; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.doc_number_counter (
    prefix text NOT NULL,
    year_month text NOT NULL,
    last_seq integer DEFAULT 0 NOT NULL
);


ALTER TABLE public.doc_number_counter OWNER TO postgres;

--
-- TOC entry 250 (class 1259 OID 20901)
-- Name: erp_audit_log; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.erp_audit_log (
    id bigint NOT NULL,
    table_name character varying(100) NOT NULL,
    record_id bigint NOT NULL,
    action character varying(10) NOT NULL,
    changed_by bigint,
    changed_at timestamp without time zone DEFAULT now() NOT NULL,
    old_data jsonb,
    new_data jsonb,
    session_info jsonb,
    CONSTRAINT erp_audit_log_action_check CHECK (((action)::text = ANY (ARRAY['INSERT'::text, 'UPDATE'::text, 'DELETE'::text])))
);


ALTER TABLE public.erp_audit_log OWNER TO postgres;

--
-- TOC entry 251 (class 1259 OID 20908)
-- Name: erp_audit_log_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.erp_audit_log ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.erp_audit_log_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- TOC entry 252 (class 1259 OID 20909)
-- Name: grn; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.grn (
    id bigint NOT NULL,
    grn_no character varying(30) NOT NULL,
    grn_date date DEFAULT CURRENT_DATE NOT NULL,
    po_id bigint NOT NULL,
    warehouse_code character varying(20) NOT NULL,
    delivery_note character varying(100),
    status character varying(20) DEFAULT 'DRAFT'::character varying NOT NULL,
    quality_status character varying(20) DEFAULT 'PENDING'::character varying NOT NULL,
    received_by bigint NOT NULL,
    confirmed_by bigint,
    confirmed_at timestamp without time zone,
    remarks text,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint,
    score_quality smallint,
    score_quantity smallint,
    score_ontime smallint,
    score_notes text,
    supplier_id integer,
    invoice_no character varying(100),
    delivery_date date,
    CONSTRAINT grn_quality_check CHECK (((quality_status)::text = ANY (ARRAY['PENDING'::text, 'PASSED'::text, 'FAILED'::text, 'PARTIAL'::text]))),
    CONSTRAINT grn_score_ontime_check CHECK (((score_ontime IS NULL) OR ((score_ontime >= 1) AND (score_ontime <= 5)))),
    CONSTRAINT grn_score_quality_check CHECK (((score_quality IS NULL) OR ((score_quality >= 1) AND (score_quality <= 5)))),
    CONSTRAINT grn_score_quantity_check CHECK (((score_quantity IS NULL) OR ((score_quantity >= 1) AND (score_quantity <= 5)))),
    CONSTRAINT grn_status_check CHECK (((status)::text = ANY (ARRAY['DRAFT'::text, 'CONFIRMED'::text, 'POSTED'::text])))
);


ALTER TABLE public.grn OWNER TO postgres;

--
-- TOC entry 5179 (class 0 OID 0)
-- Dependencies: 252
-- Name: COLUMN grn.score_quality; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.grn.score_quality IS 'คะแนนคุณภาพสินค้า 1-5 ให้หลังรับของเข้าระบบแล้ว (nullable จนกว่าจะให้คะแนน)';


--
-- TOC entry 5180 (class 0 OID 0)
-- Dependencies: 252
-- Name: COLUMN grn.score_quantity; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.grn.score_quantity IS 'คะแนนความครบถ้วนของจำนวน 1-5';


--
-- TOC entry 5181 (class 0 OID 0)
-- Dependencies: 252
-- Name: COLUMN grn.score_ontime; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.grn.score_ontime IS 'คะแนนความตรงเวลาในการส่งของ 1-5';


--
-- TOC entry 253 (class 1259 OID 20924)
-- Name: grn_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.grn ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.grn_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- TOC entry 254 (class 1259 OID 20925)
-- Name: grn_line; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.grn_line (
    id bigint NOT NULL,
    grn_id bigint NOT NULL,
    line_no integer NOT NULL,
    po_line_id bigint NOT NULL,
    mat_code character varying(20) NOT NULL,
    zone_id integer,
    qty_received numeric(18,4) NOT NULL,
    qty_accepted numeric(18,4) DEFAULT 0 NOT NULL,
    qty_rejected numeric(18,4) DEFAULT 0 NOT NULL,
    quality_remarks text,
    CONSTRAINT grn_line_qty_received_check CHECK ((qty_received >= (0)::numeric))
);


ALTER TABLE public.grn_line OWNER TO postgres;

--
-- TOC entry 255 (class 1259 OID 20933)
-- Name: grn_line_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.grn_line ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.grn_line_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- TOC entry 256 (class 1259 OID 20934)
-- Name: grn_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.grn_seq
    START WITH 10
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.grn_seq OWNER TO postgres;

--
-- TOC entry 257 (class 1259 OID 20935)
-- Name: ic_po_receive_document; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.ic_po_receive_document (
    id bigint NOT NULL,
    po_id bigint NOT NULL,
    tax_invoice_no character varying(50),
    tax_invoice_date date,
    temp_delivery_no character varying(50),
    temp_delivery_date date,
    credit_days integer,
    due_date date,
    exchange_rate numeric(18,6),
    remarks text,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint,
    receive_no character varying(30),
    score_quality numeric,
    score_quantity numeric,
    score_ontime numeric,
    score_notes text,
    rated_at timestamp with time zone,
    rated_by bigint
);


ALTER TABLE public.ic_po_receive_document OWNER TO postgres;

--
-- TOC entry 258 (class 1259 OID 20942)
-- Name: ic_po_receive_document_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.ic_po_receive_document_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.ic_po_receive_document_id_seq OWNER TO postgres;

--
-- TOC entry 5182 (class 0 OID 0)
-- Dependencies: 258
-- Name: ic_po_receive_document_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.ic_po_receive_document_id_seq OWNED BY public.ic_po_receive_document.id;


--
-- TOC entry 414 (class 1259 OID 23404)
-- Name: ic_po_return_document; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.ic_po_return_document (
    id bigint NOT NULL,
    po_id bigint NOT NULL,
    return_no character varying(30) NOT NULL,
    return_date date DEFAULT (timezone('Asia/Bangkok'::text, now()))::date NOT NULL,
    remarks text,
    created_by bigint,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL
);


ALTER TABLE public.ic_po_return_document OWNER TO postgres;

--
-- TOC entry 413 (class 1259 OID 23403)
-- Name: ic_po_return_document_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.ic_po_return_document ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.ic_po_return_document_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- TOC entry 416 (class 1259 OID 23423)
-- Name: ic_po_return_line; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.ic_po_return_line (
    id bigint NOT NULL,
    return_id bigint NOT NULL,
    line_no integer NOT NULL,
    po_line_id bigint NOT NULL,
    mat_code character varying(30) NOT NULL,
    cost_subgroup_id bigint,
    return_qty numeric(18,4) NOT NULL,
    remarks text,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    CONSTRAINT ic_po_return_line_return_qty_check CHECK ((return_qty > (0)::numeric))
);


ALTER TABLE public.ic_po_return_line OWNER TO postgres;

--
-- TOC entry 415 (class 1259 OID 23422)
-- Name: ic_po_return_line_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.ic_po_return_line ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.ic_po_return_line_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- TOC entry 259 (class 1259 OID 20943)
-- Name: ic_project_cost_item; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.ic_project_cost_item (
    id bigint NOT NULL,
    project_code character varying(20) NOT NULL,
    mat_code character varying(20) NOT NULL,
    cost_subgroup_id bigint NOT NULL,
    item_name character varying(255),
    unit character varying(50),
    qty_on_hand numeric(18,4) DEFAULT 0 NOT NULL,
    last_unit_cost numeric(18,4) DEFAULT 0 NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL
);


ALTER TABLE public.ic_project_cost_item OWNER TO postgres;

--
-- TOC entry 260 (class 1259 OID 20950)
-- Name: ic_project_cost_item_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.ic_project_cost_item_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.ic_project_cost_item_id_seq OWNER TO postgres;

--
-- TOC entry 5183 (class 0 OID 0)
-- Dependencies: 260
-- Name: ic_project_cost_item_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.ic_project_cost_item_id_seq OWNED BY public.ic_project_cost_item.id;


--
-- TOC entry 261 (class 1259 OID 20951)
-- Name: ic_project_cost_item_transaction; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.ic_project_cost_item_transaction (
    id bigint NOT NULL,
    project_cost_item_id bigint NOT NULL,
    po_id bigint,
    po_line_id bigint,
    qty numeric(18,4) NOT NULL,
    qty_before numeric(18,4) NOT NULL,
    qty_after numeric(18,4) NOT NULL,
    unit_cost numeric(18,4) NOT NULL,
    txn_date date DEFAULT CURRENT_DATE NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    remarks text,
    receive_document_id bigint,
    movement_id bigint,
    movement_line_id bigint,
    ref_type character varying(20) DEFAULT 'PO'::character varying NOT NULL,
    pr_id bigint,
    pr_line_id bigint,
    return_document_id bigint,
    CONSTRAINT chk_ic_project_cost_item_txn_ref_consistency CHECK (((((ref_type)::text = 'PO'::text) AND (po_id IS NOT NULL) AND (po_line_id IS NOT NULL) AND (movement_id IS NULL) AND (movement_line_id IS NULL) AND (pr_id IS NULL) AND (pr_line_id IS NULL)) OR (((ref_type)::text = ANY ((ARRAY['MOVEMENT_ISSUE'::character varying, 'MOVEMENT_TRANSFER'::character varying])::text[])) AND (movement_id IS NOT NULL) AND (movement_line_id IS NOT NULL) AND (po_id IS NULL) AND (po_line_id IS NULL) AND (pr_id IS NULL) AND (pr_line_id IS NULL)) OR (((ref_type)::text = 'PR'::text) AND (pr_id IS NOT NULL) AND (pr_line_id IS NOT NULL) AND (po_id IS NULL) AND (po_line_id IS NULL) AND (movement_id IS NULL) AND (movement_line_id IS NULL))))
);


ALTER TABLE public.ic_project_cost_item_transaction OWNER TO postgres;

--
-- TOC entry 262 (class 1259 OID 20960)
-- Name: ic_project_cost_item_transaction_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.ic_project_cost_item_transaction_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.ic_project_cost_item_transaction_id_seq OWNER TO postgres;

--
-- TOC entry 5184 (class 0 OID 0)
-- Dependencies: 262
-- Name: ic_project_cost_item_transaction_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.ic_project_cost_item_transaction_id_seq OWNED BY public.ic_project_cost_item_transaction.id;


--
-- TOC entry 263 (class 1259 OID 20961)
-- Name: ic_project_movement; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.ic_project_movement (
    id bigint NOT NULL,
    doc_no character varying(30) NOT NULL,
    doc_type character varying(10) NOT NULL,
    doc_date date DEFAULT CURRENT_DATE NOT NULL,
    project_code character varying(20) NOT NULL,
    job_code character varying(20) NOT NULL,
    requested_by bigint NOT NULL,
    status character varying(20) DEFAULT 'DRAFT'::character varying NOT NULL,
    remarks text,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint,
    CONSTRAINT chk_ic_project_movement_doc_type CHECK (((doc_type)::text = ANY (ARRAY[('ISSUE'::character varying)::text, ('TRANSFER'::character varying)::text])))
);


ALTER TABLE public.ic_project_movement OWNER TO postgres;

--
-- TOC entry 264 (class 1259 OID 20971)
-- Name: ic_project_movement_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.ic_project_movement ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.ic_project_movement_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- TOC entry 265 (class 1259 OID 20972)
-- Name: ic_project_movement_line; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.ic_project_movement_line (
    id bigint NOT NULL,
    movement_id bigint NOT NULL,
    line_no integer NOT NULL,
    mat_code character varying(20) NOT NULL,
    cost_subgroup_id bigint,
    qty numeric(18,4) NOT NULL,
    to_project_code character varying(20) NOT NULL,
    to_cost_subgroup_id bigint,
    remarks text,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    CONSTRAINT chk_ic_project_movement_line_qty_positive CHECK ((qty > (0)::numeric))
);


ALTER TABLE public.ic_project_movement_line OWNER TO postgres;

--
-- TOC entry 266 (class 1259 OID 20979)
-- Name: ic_project_movement_line_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.ic_project_movement_line ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.ic_project_movement_line_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- TOC entry 267 (class 1259 OID 20980)
-- Name: ic_project_receipt_pool; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.ic_project_receipt_pool (
    id bigint NOT NULL,
    project_code character varying(20) NOT NULL,
    mat_code character varying(20) NOT NULL,
    cost_subgroup_id bigint NOT NULL,
    qty_received numeric(18,4) DEFAULT 0 NOT NULL,
    qty_issued numeric(18,4) DEFAULT 0 NOT NULL,
    item_name character varying(255),
    unit character varying(20),
    last_unit_cost numeric(18,4),
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    CONSTRAINT chk_ic_project_receipt_pool_issued CHECK ((qty_issued <= qty_received))
);


ALTER TABLE public.ic_project_receipt_pool OWNER TO postgres;

--
-- TOC entry 268 (class 1259 OID 20988)
-- Name: ic_project_receipt_pool_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.ic_project_receipt_pool ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.ic_project_receipt_pool_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- TOC entry 269 (class 1259 OID 20989)
-- Name: ic_project_receipt_pool_transaction; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.ic_project_receipt_pool_transaction (
    id bigint NOT NULL,
    receipt_pool_id bigint NOT NULL,
    txn_type character varying(20) NOT NULL,
    qty numeric(18,4) NOT NULL,
    ref_type character varying(20) NOT NULL,
    po_id bigint,
    po_line_id bigint,
    receive_document_id bigint,
    movement_id bigint,
    movement_line_id bigint,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    CONSTRAINT chk_ic_project_receipt_pool_txn_type CHECK (((txn_type)::text = ANY ((ARRAY['RETURN'::character varying, 'RECEIVE'::character varying, 'ISSUE_OUT'::character varying])::text[])))
);


ALTER TABLE public.ic_project_receipt_pool_transaction OWNER TO postgres;

--
-- TOC entry 270 (class 1259 OID 20994)
-- Name: ic_project_receipt_pool_transaction_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.ic_project_receipt_pool_transaction ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.ic_project_receipt_pool_transaction_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- TOC entry 271 (class 1259 OID 20995)
-- Name: inventory; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.inventory (
    id bigint NOT NULL,
    mat_code character varying(20) NOT NULL,
    warehouse_code character varying(20) NOT NULL,
    zone_id integer,
    qty_on_hand numeric(18,4) DEFAULT 0 NOT NULL,
    qty_reserved numeric(18,4) DEFAULT 0 NOT NULL,
    qty_on_order numeric(18,4) DEFAULT 0 NOT NULL,
    reorder_point numeric(18,4),
    reorder_qty numeric(18,4),
    min_stock numeric(18,4),
    max_stock numeric(18,4),
    last_counted_at timestamp without time zone,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint
);


ALTER TABLE public.inventory OWNER TO postgres;

--
-- TOC entry 272 (class 1259 OID 21003)
-- Name: inventory_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.inventory ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.inventory_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- TOC entry 273 (class 1259 OID 21004)
-- Name: inventory_transaction; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.inventory_transaction (
    id bigint NOT NULL,
    txn_no character varying(30) NOT NULL,
    txn_type character varying(30) NOT NULL,
    mat_code character varying(20) NOT NULL,
    from_warehouse character varying(20),
    to_warehouse character varying(20),
    from_zone_id integer,
    to_zone_id integer,
    qty numeric(18,4) NOT NULL,
    ref_doc_type character varying(30),
    ref_doc_no character varying(30),
    location_code character varying(20),
    reason text,
    txn_date date DEFAULT CURRENT_DATE NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint NOT NULL,
    updated_by bigint,
    CONSTRAINT inventory_transaction_qty_check CHECK ((qty > (0)::numeric)),
    CONSTRAINT inventory_transaction_txn_type_check CHECK (((txn_type)::text = ANY (ARRAY['ISSUE'::text, 'RETURN'::text, 'TRANSFER_OUT'::text, 'TRANSFER_IN'::text, 'ADJUST_PLUS'::text, 'ADJUST_MINUS'::text, 'GRN_IN'::text, 'BORROW_OUT'::text, 'BORROW_RETURN'::text])))
);


ALTER TABLE public.inventory_transaction OWNER TO postgres;

--
-- TOC entry 274 (class 1259 OID 21014)
-- Name: inventory_transaction_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.inventory_transaction ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.inventory_transaction_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- TOC entry 275 (class 1259 OID 21015)
-- Name: location; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.location (
    id integer NOT NULL,
    location_code character varying(20) NOT NULL,
    location_name character varying(200) NOT NULL,
    location_type character varying(50) NOT NULL,
    parent_id integer,
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint,
    warehouse_code character varying(20),
    CONSTRAINT location_type_check CHECK (((location_type)::text = ANY (ARRAY['department'::text, 'project'::text, 'site'::text])))
);


ALTER TABLE public.location OWNER TO postgres;

--
-- TOC entry 276 (class 1259 OID 21022)
-- Name: location_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.location_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.location_id_seq OWNER TO postgres;

--
-- TOC entry 5185 (class 0 OID 0)
-- Dependencies: 276
-- Name: location_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.location_id_seq OWNED BY public.location.id;


--
-- TOC entry 277 (class 1259 OID 21023)
-- Name: mat_group; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.mat_group (
    id integer NOT NULL,
    group_code character varying(10) NOT NULL,
    group_name character varying(100) NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint
);


ALTER TABLE public.mat_group OWNER TO postgres;

--
-- TOC entry 278 (class 1259 OID 21029)
-- Name: mat_group_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.mat_group_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.mat_group_id_seq OWNER TO postgres;

--
-- TOC entry 5186 (class 0 OID 0)
-- Dependencies: 278
-- Name: mat_group_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.mat_group_id_seq OWNED BY public.mat_group.id;


--
-- TOC entry 279 (class 1259 OID 21030)
-- Name: mat_name; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.mat_name (
    id integer NOT NULL,
    mat_name_code character varying(10) NOT NULL,
    subgroup_id integer NOT NULL,
    mat_name character varying(200) NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint
);


ALTER TABLE public.mat_name OWNER TO postgres;

--
-- TOC entry 280 (class 1259 OID 21036)
-- Name: mat_name_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.mat_name_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.mat_name_id_seq OWNER TO postgres;

--
-- TOC entry 5187 (class 0 OID 0)
-- Dependencies: 280
-- Name: mat_name_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.mat_name_id_seq OWNED BY public.mat_name.id;


--
-- TOC entry 281 (class 1259 OID 21037)
-- Name: material_code; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.material_code (
    id integer NOT NULL,
    mat_code character varying(20) NOT NULL,
    group_id integer NOT NULL,
    subgroup_id integer NOT NULL,
    mat_name_id integer NOT NULL,
    spec_id integer NOT NULL,
    brand_id integer NOT NULL,
    unit_id integer NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint,
    cost_subgroup_id integer
);


ALTER TABLE public.material_code OWNER TO postgres;

--
-- TOC entry 282 (class 1259 OID 21043)
-- Name: material_code_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.material_code_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.material_code_id_seq OWNER TO postgres;

--
-- TOC entry 5188 (class 0 OID 0)
-- Dependencies: 282
-- Name: material_code_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.material_code_id_seq OWNED BY public.material_code.id;


--
-- TOC entry 283 (class 1259 OID 21044)
-- Name: memo; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.memo (
    id bigint NOT NULL,
    memo_no character varying(30) NOT NULL,
    title character varying(255),
    project_code character varying(20),
    requested_by bigint NOT NULL,
    department character varying(100),
    note text,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint,
    status character varying(30) DEFAULT 'DRAFT'::character varying NOT NULL,
    approver_id bigint,
    delivery_location text,
    site_delivery_date date,
    responsible_factory character varying(255),
    job_code character varying(20),
    CONSTRAINT memo_status_check CHECK (((status)::text = ANY (ARRAY['DRAFT'::text, 'PENDING_APPROVAL'::text, 'APPROVED'::text, 'REJECTED'::text, 'CANCELLED'::text])))
);


ALTER TABLE public.memo OWNER TO postgres;

--
-- TOC entry 5189 (class 0 OID 0)
-- Dependencies: 283
-- Name: COLUMN memo.status; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.memo.status IS 'DRAFT → PENDING_APPROVAL → APPROVED/REJECTED';


--
-- TOC entry 5190 (class 0 OID 0)
-- Dependencies: 283
-- Name: COLUMN memo.approver_id; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.memo.approver_id IS 'ผู้อนุมัติที่ถูกกำหนดไว้ตอนสร้าง Memo — เฉพาะคนนี้เท่านั้นที่ approve ได้';


--
-- TOC entry 284 (class 1259 OID 21053)
-- Name: memo_attachment; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.memo_attachment (
    id bigint NOT NULL,
    memo_id bigint NOT NULL,
    file_path text NOT NULL,
    file_name character varying(255) NOT NULL,
    file_size bigint NOT NULL,
    file_type character varying(20),
    uploaded_by bigint,
    uploaded_at timestamp without time zone DEFAULT now() NOT NULL
);


ALTER TABLE public.memo_attachment OWNER TO postgres;

--
-- TOC entry 5191 (class 0 OID 0)
-- Dependencies: 284
-- Name: TABLE memo_attachment; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON TABLE public.memo_attachment IS 'ไฟล์แนบของ Memo — รูปภาพ, Excel, CSV, PDF';


--
-- TOC entry 285 (class 1259 OID 21059)
-- Name: memo_attachment_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.memo_attachment ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.memo_attachment_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- TOC entry 286 (class 1259 OID 21060)
-- Name: memo_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.memo ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.memo_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- TOC entry 287 (class 1259 OID 21061)
-- Name: memo_line; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.memo_line (
    id bigint NOT NULL,
    memo_id bigint NOT NULL,
    line_no integer NOT NULL,
    description text NOT NULL,
    unit character varying(50) NOT NULL,
    quantity numeric(18,4) NOT NULL,
    remark text,
    CONSTRAINT memo_line_qty_check CHECK ((quantity > (0)::numeric))
);


ALTER TABLE public.memo_line OWNER TO postgres;

--
-- TOC entry 288 (class 1259 OID 21067)
-- Name: memo_line_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.memo_line ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.memo_line_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- TOC entry 289 (class 1259 OID 21068)
-- Name: memo_number_counter; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.memo_number_counter (
    year_month character varying(6) NOT NULL,
    last_seq integer DEFAULT 0 NOT NULL
);


ALTER TABLE public.memo_number_counter OWNER TO postgres;

--
-- TOC entry 290 (class 1259 OID 21072)
-- Name: memo_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.memo_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.memo_seq OWNER TO postgres;

--
-- TOC entry 291 (class 1259 OID 21073)
-- Name: memo_status_log; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.memo_status_log (
    id bigint NOT NULL,
    memo_id bigint NOT NULL,
    from_status character varying(30),
    to_status character varying(30) NOT NULL,
    changed_by bigint NOT NULL,
    changed_at timestamp without time zone DEFAULT now() NOT NULL,
    remarks text
);


ALTER TABLE public.memo_status_log OWNER TO postgres;

--
-- TOC entry 5192 (class 0 OID 0)
-- Dependencies: 291
-- Name: TABLE memo_status_log; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON TABLE public.memo_status_log IS 'ประวัติการเปลี่ยนสถานะของ Memo';


--
-- TOC entry 292 (class 1259 OID 21079)
-- Name: memo_status_log_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.memo_status_log ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.memo_status_log_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- TOC entry 293 (class 1259 OID 21080)
-- Name: menus; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.menus (
    id bigint NOT NULL,
    parent_id bigint,
    menu_code character varying(100) NOT NULL,
    menu_name character varying(255) NOT NULL,
    menu_path character varying(255),
    icon_name character varying(100),
    sort_order integer DEFAULT 0,
    is_active boolean DEFAULT true,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint
);


ALTER TABLE public.menus OWNER TO postgres;

--
-- TOC entry 294 (class 1259 OID 21089)
-- Name: menus_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.menus ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.menus_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- TOC entry 295 (class 1259 OID 21090)
-- Name: modules; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.modules (
    id bigint NOT NULL,
    module_code character varying(100) NOT NULL,
    module_name character varying(255) NOT NULL,
    sort_order integer DEFAULT 0,
    is_active boolean DEFAULT true,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint
);


ALTER TABLE public.modules OWNER TO postgres;

--
-- TOC entry 296 (class 1259 OID 21097)
-- Name: modules_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.modules ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.modules_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- TOC entry 426 (class 1259 OID 25577)
-- Name: payment_installment; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.payment_installment (
    id bigint NOT NULL,
    doc_type character varying(10) NOT NULL,
    doc_id bigint NOT NULL,
    installment_no integer NOT NULL,
    percent numeric(7,4),
    amount numeric(18,2) DEFAULT 0 NOT NULL,
    description text,
    is_paid boolean DEFAULT false NOT NULL,
    paid_at timestamp without time zone,
    paid_by bigint,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint,
    payment_log_id bigint,
    CONSTRAINT payment_installment_amount_check CHECK ((amount >= (0)::numeric)),
    CONSTRAINT payment_installment_doc_type_check CHECK (((doc_type)::text = ANY ((ARRAY['PO'::character varying, 'WO'::character varying, 'PC'::character varying])::text[]))),
    CONSTRAINT payment_installment_percent_check CHECK (((percent IS NULL) OR ((percent >= (0)::numeric) AND (percent <= (100)::numeric))))
);


ALTER TABLE public.payment_installment OWNER TO postgres;

--
-- TOC entry 425 (class 1259 OID 25576)
-- Name: payment_installment_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.payment_installment ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.payment_installment_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- TOC entry 297 (class 1259 OID 21098)
-- Name: payment_log; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.payment_log (
    id bigint NOT NULL,
    doc_type character varying(30) NOT NULL,
    doc_id bigint NOT NULL,
    doc_no character varying(30) NOT NULL,
    amount_paid numeric(18,2) NOT NULL,
    paid_date date DEFAULT CURRENT_DATE NOT NULL,
    paid_by bigint NOT NULL,
    remark text,
    reverses_id bigint,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint NOT NULL,
    CONSTRAINT payment_log_amount_paid_check CHECK ((amount_paid <> (0)::numeric)),
    CONSTRAINT payment_log_amount_sign_chk CHECK ((((reverses_id IS NULL) AND (amount_paid > (0)::numeric)) OR ((reverses_id IS NOT NULL) AND (amount_paid < (0)::numeric)))),
    CONSTRAINT payment_log_doc_type_check CHECK (((doc_type)::text = ANY (ARRAY[('PO'::character varying)::text, ('WO'::character varying)::text, ('PC'::character varying)::text])))
);


ALTER TABLE public.payment_log OWNER TO postgres;

--
-- TOC entry 298 (class 1259 OID 21107)
-- Name: payment_log_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.payment_log ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.payment_log_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- TOC entry 417 (class 1259 OID 23668)
-- Name: pc_number_counter; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.pc_number_counter (
    year_month character varying(6) NOT NULL,
    last_seq integer DEFAULT 0 NOT NULL
);


ALTER TABLE public.pc_number_counter OWNER TO postgres;

--
-- TOC entry 299 (class 1259 OID 21108)
-- Name: permission_audit_logs; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.permission_audit_logs (
    id bigint NOT NULL,
    user_id bigint,
    action character varying(100),
    target_type character varying(100),
    target_id bigint,
    old_value jsonb,
    new_value jsonb,
    created_at timestamp without time zone DEFAULT now() NOT NULL
);


ALTER TABLE public.permission_audit_logs OWNER TO postgres;

--
-- TOC entry 300 (class 1259 OID 21114)
-- Name: permission_audit_logs_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.permission_audit_logs ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.permission_audit_logs_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- TOC entry 301 (class 1259 OID 21115)
-- Name: permissions; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.permissions (
    id bigint NOT NULL,
    permission_key character varying(200) NOT NULL,
    permission_name character varying(255),
    module_id bigint NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint
);


ALTER TABLE public.permissions OWNER TO postgres;

--
-- TOC entry 302 (class 1259 OID 21120)
-- Name: permissions_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.permissions ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.permissions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- TOC entry 419 (class 1259 OID 23687)
-- Name: petty_cash_attachment; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.petty_cash_attachment (
    id bigint NOT NULL,
    pc_id bigint NOT NULL,
    file_name character varying(255) NOT NULL,
    file_path text NOT NULL,
    file_size bigint,
    file_type character varying(100),
    uploaded_at timestamp without time zone DEFAULT now() NOT NULL,
    uploaded_by bigint
);


ALTER TABLE public.petty_cash_attachment OWNER TO postgres;

--
-- TOC entry 418 (class 1259 OID 23686)
-- Name: petty_cash_attachment_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.petty_cash_attachment ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.petty_cash_attachment_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- TOC entry 303 (class 1259 OID 21121)
-- Name: petty_cash_requisition; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.petty_cash_requisition (
    id bigint NOT NULL,
    pc_no character varying(30) NOT NULL,
    pc_date date DEFAULT CURRENT_DATE NOT NULL,
    requested_by bigint NOT NULL,
    purpose text,
    currency character varying(10) DEFAULT 'THB'::character varying NOT NULL,
    total_amount numeric(18,4) DEFAULT 0 NOT NULL,
    use_discount boolean DEFAULT false NOT NULL,
    discount_type character varying(10) DEFAULT 'pct'::character varying NOT NULL,
    discount_amount numeric(18,4) DEFAULT 0 NOT NULL,
    use_vat boolean DEFAULT false NOT NULL,
    vat_amount numeric(18,4) DEFAULT 0 NOT NULL,
    use_wht boolean DEFAULT false NOT NULL,
    wht_amount numeric(18,4) DEFAULT 0 NOT NULL,
    net_amount numeric(18,4) DEFAULT 0 NOT NULL,
    status character varying(20) DEFAULT 'DRAFT'::character varying NOT NULL,
    approver_id bigint,
    remarks text,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint NOT NULL,
    updated_by bigint,
    doc_type character varying(30) DEFAULT 'PETTY_CASH'::character varying NOT NULL,
    branch_code character varying(20),
    payee_emp_code character varying(30),
    payee_name character varying(200),
    project_code character varying(20),
    project_name character varying(200),
    order_type character varying(30),
    dept_code character varying(20),
    dept_name character varying(200),
    ref_doc_no character varying(50),
    approver_remarks text,
    CONSTRAINT chk_pc_discount_type CHECK (((discount_type)::text = ANY (ARRAY[('pct'::character varying)::text, ('amt'::character varying)::text]))),
    CONSTRAINT chk_pc_status CHECK (((status)::text = ANY (ARRAY[('DRAFT'::character varying)::text, ('PENDING_APPROVAL'::character varying)::text, ('APPROVED'::character varying)::text, ('REJECTED'::character varying)::text, ('CANCELLED'::character varying)::text]))),
    CONSTRAINT petty_cash_requisition_order_type_check CHECK (((order_type IS NULL) OR ((order_type)::text = ANY ((ARRAY['stock'::character varying, 'cost'::character varying, 'asset_machine'::character varying, 'asset_office_equipment'::character varying, 'asset_tools'::character varying])::text[]))))
);


ALTER TABLE public.petty_cash_requisition OWNER TO postgres;

--
-- TOC entry 304 (class 1259 OID 21142)
-- Name: petty_cash_requisition_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.petty_cash_requisition ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.petty_cash_requisition_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- TOC entry 305 (class 1259 OID 21143)
-- Name: petty_cash_requisition_line; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.petty_cash_requisition_line (
    id bigint NOT NULL,
    pc_id bigint NOT NULL,
    line_no integer NOT NULL,
    mat_code character varying(20),
    cost_subgroup_id bigint,
    description text,
    qty numeric(18,4) NOT NULL,
    unit_price numeric(18,4) NOT NULL,
    amount numeric(18,4),
    discount numeric(18,4) DEFAULT 0 NOT NULL,
    disc_type character varying(10) DEFAULT 'pct'::character varying NOT NULL,
    line_discount numeric(18,4) DEFAULT 0 NOT NULL,
    line_vat numeric(18,4) DEFAULT 0 NOT NULL,
    line_wht numeric(18,4) DEFAULT 0 NOT NULL,
    line_net numeric(18,4) DEFAULT 0 NOT NULL,
    wht_rate numeric(5,2),
    remarks text,
    project_code character varying(20),
    has_tax_invoice boolean DEFAULT false NOT NULL,
    has_wht boolean DEFAULT false NOT NULL,
    supplier_id integer,
    supplier_name character varying(200),
    supplier_tax_id character varying(50),
    supplier_address text,
    tax_form_code character varying(20),
    tax_invoice_no character varying(50),
    tax_invoice_date date,
    wht_income_type_id integer,
    wht_base_amount numeric(18,4),
    wht_pay_date date,
    invoice_no character varying(50),
    invoice_date date,
    CONSTRAINT chk_pcl_disc_type CHECK (((disc_type)::text = ANY (ARRAY[('pct'::character varying)::text, ('amt'::character varying)::text]))),
    CONSTRAINT chk_pcl_name_or_code CHECK (((mat_code IS NOT NULL) OR (length(btrim(COALESCE(description, ''::text))) > 0))),
    CONSTRAINT chk_pcl_tax_form CHECK (((tax_form_code IS NULL) OR ((tax_form_code)::text = ANY (ARRAY[('PND3'::character varying)::text, ('PND53'::character varying)::text])))),
    CONSTRAINT chk_pcl_wht_fields CHECK (((NOT has_wht) OR ((tax_form_code IS NOT NULL) AND (wht_income_type_id IS NOT NULL) AND (wht_rate IS NOT NULL))))
);


ALTER TABLE public.petty_cash_requisition_line OWNER TO postgres;

--
-- TOC entry 306 (class 1259 OID 21155)
-- Name: petty_cash_requisition_line_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.petty_cash_requisition_line ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.petty_cash_requisition_line_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- TOC entry 307 (class 1259 OID 21156)
-- Name: petty_cash_status_log; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.petty_cash_status_log (
    id bigint NOT NULL,
    pc_id bigint NOT NULL,
    from_status character varying(30),
    to_status character varying(30) NOT NULL,
    changed_by bigint NOT NULL,
    changed_at timestamp without time zone DEFAULT now() NOT NULL,
    remarks text
);


ALTER TABLE public.petty_cash_status_log OWNER TO postgres;

--
-- TOC entry 308 (class 1259 OID 21162)
-- Name: petty_cash_status_log_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.petty_cash_status_log ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.petty_cash_status_log_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- TOC entry 309 (class 1259 OID 21163)
-- Name: po_attachment; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.po_attachment (
    id integer NOT NULL,
    po_id bigint NOT NULL,
    file_name character varying NOT NULL,
    file_path character varying NOT NULL,
    file_size bigint,
    file_type character varying,
    uploaded_at timestamp without time zone DEFAULT now(),
    uploaded_by bigint,
    source_pr_attachment_id bigint
);


ALTER TABLE public.po_attachment OWNER TO postgres;

--
-- TOC entry 310 (class 1259 OID 21169)
-- Name: po_attachment_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.po_attachment ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.po_attachment_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- TOC entry 311 (class 1259 OID 21170)
-- Name: po_edit_log; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.po_edit_log (
    id bigint NOT NULL,
    po_id bigint NOT NULL,
    edited_by bigint NOT NULL,
    reason text NOT NULL,
    edited_at timestamp without time zone DEFAULT now() NOT NULL
);


ALTER TABLE public.po_edit_log OWNER TO postgres;

--
-- TOC entry 312 (class 1259 OID 21176)
-- Name: po_edit_log_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.po_edit_log ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.po_edit_log_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- TOC entry 313 (class 1259 OID 21177)
-- Name: po_number_counter; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.po_number_counter (
    year_month character varying(6) NOT NULL,
    last_seq integer DEFAULT 0 NOT NULL
);


ALTER TABLE public.po_number_counter OWNER TO postgres;

--
-- TOC entry 314 (class 1259 OID 21181)
-- Name: po_status_log; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.po_status_log (
    id bigint NOT NULL,
    po_id bigint NOT NULL,
    from_status character varying(30),
    to_status character varying(30) NOT NULL,
    changed_by bigint NOT NULL,
    changed_at timestamp without time zone DEFAULT now() NOT NULL,
    remarks text
);


ALTER TABLE public.po_status_log OWNER TO postgres;

--
-- TOC entry 315 (class 1259 OID 21187)
-- Name: po_status_log_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.po_status_log ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.po_status_log_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- TOC entry 316 (class 1259 OID 21188)
-- Name: por_number_counter; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.por_number_counter (
    year_month character varying(6) NOT NULL,
    last_seq integer DEFAULT 0 NOT NULL
);


ALTER TABLE public.por_number_counter OWNER TO postgres;

--
-- TOC entry 317 (class 1259 OID 21192)
-- Name: pr_attachment; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.pr_attachment (
    id integer NOT NULL,
    pr_id bigint NOT NULL,
    file_name character varying(255) NOT NULL,
    file_path character varying(500) NOT NULL,
    file_size bigint,
    file_type character varying(100),
    uploaded_at timestamp without time zone DEFAULT now() NOT NULL,
    uploaded_by bigint
);


ALTER TABLE public.pr_attachment OWNER TO postgres;

--
-- TOC entry 318 (class 1259 OID 21198)
-- Name: pr_attachment_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.pr_attachment_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.pr_attachment_id_seq OWNER TO postgres;

--
-- TOC entry 5193 (class 0 OID 0)
-- Dependencies: 318
-- Name: pr_attachment_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.pr_attachment_id_seq OWNED BY public.pr_attachment.id;


--
-- TOC entry 319 (class 1259 OID 21199)
-- Name: pr_number_counter; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.pr_number_counter (
    year_month character varying(6) NOT NULL,
    last_seq integer DEFAULT 0 NOT NULL
);


ALTER TABLE public.pr_number_counter OWNER TO postgres;

--
-- TOC entry 320 (class 1259 OID 21203)
-- Name: pr_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.pr_seq
    START WITH 10
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.pr_seq OWNER TO postgres;

--
-- TOC entry 321 (class 1259 OID 21204)
-- Name: pr_status_log; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.pr_status_log (
    id bigint NOT NULL,
    pr_id bigint NOT NULL,
    from_status character varying(30),
    to_status character varying(30) NOT NULL,
    changed_by bigint NOT NULL,
    changed_at timestamp without time zone DEFAULT now() NOT NULL,
    remarks text
);


ALTER TABLE public.pr_status_log OWNER TO postgres;

--
-- TOC entry 322 (class 1259 OID 21210)
-- Name: pr_status_log_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.pr_status_log ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.pr_status_log_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- TOC entry 323 (class 1259 OID 21211)
-- Name: project; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.project (
    id integer NOT NULL,
    project_code character varying(20) NOT NULL,
    project_name character varying(200) NOT NULL,
    location_code character varying(500),
    start_date date,
    end_date date,
    status character varying(20) DEFAULT 'ACTIVE'::character varying NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint,
    owner_id bigint,
    budget_amount numeric(18,4) DEFAULT 0 NOT NULL,
    consultant_name character varying(255),
    job_codes text[],
    owner_name character varying(200),
    credit character varying(200),
    dept_code character varying(20),
    responsible_person_name character varying(200),
    consultant_phone character varying(50),
    customer_id bigint,
    warehouse_code character varying(50),
    linked_warehouse_code character varying(20),
    project_kind character varying(10) DEFAULT 'NEW'::character varying NOT NULL,
    contract_amount numeric(18,4) DEFAULT 0 NOT NULL,
    cost_amount numeric(18,4) DEFAULT 0 NOT NULL,
    vo_amount numeric(18,4) DEFAULT 0 NOT NULL,
    CONSTRAINT project_kind_check CHECK (((project_kind)::text = ANY ((ARRAY['NEW'::character varying, 'VO'::character varying])::text[]))),
    CONSTRAINT project_status_check CHECK (((status)::text = ANY (ARRAY[('ACTIVE'::character varying)::text, ('INACTIVE'::character varying)::text, ('CLOSED'::character varying)::text])))
);


ALTER TABLE public.project OWNER TO postgres;

--
-- TOC entry 5194 (class 0 OID 0)
-- Dependencies: 323
-- Name: COLUMN project.owner_id; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.project.owner_id IS 'ผู้รับผิดชอบหลักของโครงการ — ปกติเป็นทีมวิศวกร';


--
-- TOC entry 5195 (class 0 OID 0)
-- Dependencies: 323
-- Name: COLUMN project.budget_amount; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.project.budget_amount IS 'มูลค่าโครงการ (งบประมาณ)';


--
-- TOC entry 5196 (class 0 OID 0)
-- Dependencies: 323
-- Name: COLUMN project.consultant_name; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.project.consultant_name IS 'ที่ปรึกษาโครงการ (กรอกอิสระ)';


--
-- TOC entry 5197 (class 0 OID 0)
-- Dependencies: 323
-- Name: COLUMN project.dept_code; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.project.dept_code IS 'แผนก — FK to departments.dept_code';


--
-- TOC entry 5198 (class 0 OID 0)
-- Dependencies: 323
-- Name: COLUMN project.responsible_person_name; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.project.responsible_person_name IS 'ผู้รับผิดชอบหลัก — free text, required at the application layer; replaces the owner_id dropdown (owner_id column kept, unused)';


--
-- TOC entry 5199 (class 0 OID 0)
-- Dependencies: 323
-- Name: COLUMN project.consultant_phone; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.project.consultant_phone IS 'เบอร์ติดต่อของที่ปรึกษา — freeform, no validation';


--
-- TOC entry 427 (class 1259 OID 25648)
-- Name: project_code_counter; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.project_code_counter (
    code_year integer NOT NULL,
    cus_id integer NOT NULL,
    last_no integer DEFAULT 0 NOT NULL
);


ALTER TABLE public.project_code_counter OWNER TO postgres;

--
-- TOC entry 429 (class 1259 OID 25797)
-- Name: project_code_year_counter; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.project_code_year_counter (
    code_year integer NOT NULL,
    last_no integer DEFAULT 0 NOT NULL
);


ALTER TABLE public.project_code_year_counter OWNER TO postgres;

--
-- TOC entry 432 (class 1259 OID 25850)
-- Name: project_cost_budget; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.project_cost_budget (
    id bigint NOT NULL,
    project_id integer NOT NULL,
    cost_subgroup_id integer NOT NULL,
    budget_amount numeric(18,2) DEFAULT 0 NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint,
    CONSTRAINT chk_project_cost_budget_amount_nonneg CHECK ((budget_amount >= (0)::numeric))
);


ALTER TABLE public.project_cost_budget OWNER TO postgres;

--
-- TOC entry 431 (class 1259 OID 25849)
-- Name: project_cost_budget_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.project_cost_budget ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.project_cost_budget_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- TOC entry 324 (class 1259 OID 21222)
-- Name: project_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.project_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.project_id_seq OWNER TO postgres;

--
-- TOC entry 5200 (class 0 OID 0)
-- Dependencies: 324
-- Name: project_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.project_id_seq OWNED BY public.project.id;


--
-- TOC entry 430 (class 1259 OID 25809)
-- Name: project_job_share; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.project_job_share (
    project_id bigint NOT NULL,
    job_code character varying(10) NOT NULL,
    share_percent numeric(5,2) NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    CONSTRAINT project_job_share_share_percent_check CHECK (((share_percent > (0)::numeric) AND (share_percent <= (100)::numeric)))
);


ALTER TABLE public.project_job_share OWNER TO postgres;

--
-- TOC entry 428 (class 1259 OID 25768)
-- Name: project_responsible_person; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.project_responsible_person (
    project_id bigint NOT NULL,
    user_id bigint NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    created_by bigint
);


ALTER TABLE public.project_responsible_person OWNER TO postgres;

--
-- TOC entry 325 (class 1259 OID 21223)
-- Name: project_stock; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.project_stock (
    id bigint NOT NULL,
    project_code character varying(20) NOT NULL,
    mat_code character varying(30) NOT NULL,
    unit character varying(50),
    qty_on_hand numeric(18,4) DEFAULT 0 NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL
);


ALTER TABLE public.project_stock OWNER TO postgres;

--
-- TOC entry 326 (class 1259 OID 21228)
-- Name: project_stock_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.project_stock ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.project_stock_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- TOC entry 327 (class 1259 OID 21229)
-- Name: purchase_order; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.purchase_order (
    id bigint NOT NULL,
    po_no character varying(30) NOT NULL,
    po_date date DEFAULT CURRENT_DATE NOT NULL,
    pr_id bigint,
    rfq_id bigint,
    currency character varying(10) DEFAULT 'THB'::character varying NOT NULL,
    total_amount numeric(18,4) DEFAULT 0 NOT NULL,
    vat_amount numeric(18,4) DEFAULT 0 NOT NULL,
    net_amount numeric(18,4) DEFAULT 0 NOT NULL,
    expected_date date,
    status character varying(30) DEFAULT 'DRAFT'::character varying NOT NULL,
    payment_terms character varying(100),
    delivery_address text,
    remarks text,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint NOT NULL,
    updated_by bigint,
    use_discount boolean DEFAULT false NOT NULL,
    discount_type character varying(10) DEFAULT 'pct'::character varying NOT NULL,
    use_vat boolean DEFAULT false NOT NULL,
    use_wht boolean DEFAULT false NOT NULL,
    discount_amount numeric(18,4) DEFAULT 0 NOT NULL,
    wht_amount numeric(18,4) DEFAULT 0 NOT NULL,
    location_code character varying(20),
    location_text character varying(255),
    warehouse_code character varying(20),
    project_code character varying(20),
    requested_by bigint,
    approver_id bigint,
    ref character varying,
    status_receive character varying(20) DEFAULT 'NOT_SENT'::character varying NOT NULL,
    order_type character varying(30) DEFAULT 'stock'::character varying NOT NULL,
    job_code character varying(10) NOT NULL,
    supplier_id integer,
    receiver_name character varying(100),
    receiver_phone character varying(20),
    deleted_at timestamp with time zone,
    deleted_by bigint,
    deposit_paid boolean DEFAULT false NOT NULL,
    deposit_paid_at timestamp without time zone,
    deposit_paid_by bigint,
    CONSTRAINT po_discount_type_check CHECK (((discount_type)::text = ANY (ARRAY[('pct'::character varying)::text, ('amt'::character varying)::text]))),
    CONSTRAINT po_status_check CHECK (((status)::text = ANY (ARRAY['DRAFT'::text, 'PENDING_APPROVAL'::text, 'APPROVED'::text, 'REJECTED'::text, 'PENDING_REAPPROVAL'::text, 'SENT'::text, 'PARTIALLY_RECEIVED'::text, 'RECEIVED'::text, 'CANCELLED'::text]))),
    CONSTRAINT purchase_order_job_code_check CHECK (((job_code)::text = ANY (ARRAY[('MP'::character varying)::text, ('ME'::character varying)::text, ('MS'::character varying)::text, ('MF'::character varying)::text, ('MG'::character varying)::text, ('MH'::character varying)::text, ('FS'::character varying)::text, ('FP'::character varying)::text, ('FB'::character varying)::text, ('DE'::character varying)::text, ('RE'::character varying)::text, ('G'::character varying)::text]))),
    CONSTRAINT purchase_order_order_type_check CHECK (((order_type)::text = ANY ((ARRAY['stock'::character varying, 'cost'::character varying, 'office_equipment'::character varying, 'asset_equipment'::character varying, 'asset_tool'::character varying, 'asset_office_equipment'::character varying, 'asset_machine'::character varying, 'asset_tools'::character varying])::text[]))),
    CONSTRAINT purchase_order_status_receive_check CHECK (((status_receive)::text = ANY (ARRAY[('NOT_SENT'::character varying)::text, ('SENT'::character varying)::text, ('PARTIALLY_RECEIVED'::character varying)::text, ('RECEIVED'::character varying)::text])))
);


ALTER TABLE public.purchase_order OWNER TO postgres;

--
-- TOC entry 5201 (class 0 OID 0)
-- Dependencies: 327
-- Name: COLUMN purchase_order.use_discount; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.purchase_order.use_discount IS 'เปิดใช้ส่วนลดหรือไม่';


--
-- TOC entry 5202 (class 0 OID 0)
-- Dependencies: 327
-- Name: COLUMN purchase_order.discount_type; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.purchase_order.discount_type IS 'ประเภทส่วนลด: pct = % | amt = ฿';


--
-- TOC entry 5203 (class 0 OID 0)
-- Dependencies: 327
-- Name: COLUMN purchase_order.use_vat; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.purchase_order.use_vat IS 'เปิดใช้ VAT 7% หรือไม่';


--
-- TOC entry 5204 (class 0 OID 0)
-- Dependencies: 327
-- Name: COLUMN purchase_order.use_wht; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.purchase_order.use_wht IS 'เปิดใช้ WHT หรือไม่ (อัตราอยู่ที่แต่ละ line)';


--
-- TOC entry 5205 (class 0 OID 0)
-- Dependencies: 327
-- Name: COLUMN purchase_order.discount_amount; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.purchase_order.discount_amount IS 'ส่วนลดรวมทุก line';


--
-- TOC entry 5206 (class 0 OID 0)
-- Dependencies: 327
-- Name: COLUMN purchase_order.wht_amount; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.purchase_order.wht_amount IS 'WHT รวมทุก line';


--
-- TOC entry 5207 (class 0 OID 0)
-- Dependencies: 327
-- Name: COLUMN purchase_order.location_text; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.purchase_order.location_text IS 'สถานที่ส่งของ — free text, no longer linked to location master';


--
-- TOC entry 5208 (class 0 OID 0)
-- Dependencies: 327
-- Name: COLUMN purchase_order.warehouse_code; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.purchase_order.warehouse_code IS 'คลังสินค้า (optional) — FK to warehouse.warehouse_code, reference only';


--
-- TOC entry 328 (class 1259 OID 21255)
-- Name: purchase_order_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.purchase_order ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.purchase_order_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- TOC entry 329 (class 1259 OID 21256)
-- Name: purchase_order_line; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.purchase_order_line (
    id bigint NOT NULL,
    po_id bigint NOT NULL,
    line_no integer NOT NULL,
    mat_code character varying(20) NOT NULL,
    pr_line_id bigint,
    qty_ordered numeric(18,4) NOT NULL,
    qty_received numeric(18,4) DEFAULT 0 NOT NULL,
    unit_price numeric(18,4) NOT NULL,
    amount numeric(18,4) GENERATED ALWAYS AS ((qty_ordered * unit_price)) STORED,
    remarks text,
    status character varying(20) DEFAULT 'OPEN'::character varying NOT NULL,
    description text,
    discount numeric(18,4) DEFAULT 0 NOT NULL,
    wht_rate numeric(5,2),
    line_discount numeric(18,4) DEFAULT 0 NOT NULL,
    line_vat numeric(18,4) DEFAULT 0 NOT NULL,
    line_wht numeric(18,4) DEFAULT 0 NOT NULL,
    line_net numeric(18,4) DEFAULT 0 NOT NULL,
    disc_type character varying(3) DEFAULT 'pct'::character varying NOT NULL,
    cost_subgroup_id bigint,
    CONSTRAINT po_line_status_check CHECK (((status)::text = ANY (ARRAY['OPEN'::text, 'PARTIAL'::text, 'RECEIVED'::text, 'CANCELLED'::text]))),
    CONSTRAINT po_line_wht_rate_check CHECK (((wht_rate IS NULL) OR (wht_rate = ANY (ARRAY[(1)::numeric, (3)::numeric, (5)::numeric])))),
    CONSTRAINT purchase_order_line_qty_ordered_check CHECK ((qty_ordered > (0)::numeric)),
    CONSTRAINT purchase_order_line_unit_price_check CHECK ((unit_price >= (0)::numeric))
);


ALTER TABLE public.purchase_order_line OWNER TO postgres;

--
-- TOC entry 5209 (class 0 OID 0)
-- Dependencies: 329
-- Name: COLUMN purchase_order_line.discount; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.purchase_order_line.discount IS 'ค่าส่วนลดที่กรอก (% หรือ ฿ ตาม po.discount_type)';


--
-- TOC entry 5210 (class 0 OID 0)
-- Dependencies: 329
-- Name: COLUMN purchase_order_line.wht_rate; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.purchase_order_line.wht_rate IS 'อัตรา WHT ต่อ row: 1, 3, หรือ 5 (NULL = ไม่หัก)';


--
-- TOC entry 5211 (class 0 OID 0)
-- Dependencies: 329
-- Name: COLUMN purchase_order_line.line_discount; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.purchase_order_line.line_discount IS 'ส่วนลดเป็น ฿ ที่คำนวณแล้ว';


--
-- TOC entry 5212 (class 0 OID 0)
-- Dependencies: 329
-- Name: COLUMN purchase_order_line.line_vat; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.purchase_order_line.line_vat IS 'VAT เป็น ฿ ที่คำนวณแล้ว (7% คงที่)';


--
-- TOC entry 5213 (class 0 OID 0)
-- Dependencies: 329
-- Name: COLUMN purchase_order_line.line_wht; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.purchase_order_line.line_wht IS 'WHT เป็น ฿ ที่คำนวณแล้ว';


--
-- TOC entry 5214 (class 0 OID 0)
-- Dependencies: 329
-- Name: COLUMN purchase_order_line.line_net; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.purchase_order_line.line_net IS 'ยอดสุทธิต่อ row = amount - line_discount + line_vat - line_wht';


--
-- TOC entry 330 (class 1259 OID 21274)
-- Name: purchase_order_line_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.purchase_order_line ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.purchase_order_line_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- TOC entry 331 (class 1259 OID 21275)
-- Name: purchase_request; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.purchase_request (
    id bigint NOT NULL,
    pr_no character varying(30) NOT NULL,
    pr_date date DEFAULT CURRENT_DATE NOT NULL,
    requested_by bigint NOT NULL,
    warehouse_code character varying(20),
    required_date date,
    status character varying(30) DEFAULT 'DRAFT'::character varying NOT NULL,
    priority character varying(20) DEFAULT 'NORMAL'::character varying,
    remarks text,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint,
    project_code character varying(20),
    memo_id bigint,
    location_text character varying(255),
    order_type character varying(30) DEFAULT 'stock'::character varying NOT NULL,
    pr_type character varying(10) DEFAULT 'PO_WO'::character varying NOT NULL,
    job_code character varying(10) NOT NULL,
    dept_code character varying(50),
    deleted_at timestamp with time zone,
    deleted_by bigint,
    CONSTRAINT pr_priority_check CHECK (((priority)::text = ANY (ARRAY['LOW'::text, 'NORMAL'::text, 'HIGH'::text, 'URGENT'::text]))),
    CONSTRAINT pr_status_check CHECK (((status)::text = ANY (ARRAY['DRAFT'::text, 'COMPLETED'::text, 'STOCK_CHECK'::text, 'PARTIALLY_FILLED'::text, 'FULFILLED'::text, 'CANCELLED'::text]))),
    CONSTRAINT purchase_request_job_code_check CHECK (((job_code)::text = ANY (ARRAY[('MP'::character varying)::text, ('ME'::character varying)::text, ('MS'::character varying)::text, ('MF'::character varying)::text, ('MG'::character varying)::text, ('MH'::character varying)::text, ('FS'::character varying)::text, ('FP'::character varying)::text, ('FB'::character varying)::text, ('DE'::character varying)::text, ('RE'::character varying)::text, ('G'::character varying)::text]))),
    CONSTRAINT purchase_request_order_type_check CHECK (((order_type)::text = ANY ((ARRAY['stock'::character varying, 'cost'::character varying, 'office_equipment'::character varying, 'asset_equipment'::character varying, 'asset_tool'::character varying, 'asset_office_equipment'::character varying, 'asset_machine'::character varying, 'asset_tools'::character varying])::text[]))),
    CONSTRAINT purchase_request_pr_type_check CHECK (((pr_type)::text = ANY (ARRAY[('PO_WO'::character varying)::text, ('PO_ONLY'::character varying)::text, ('WO_ONLY'::character varying)::text])))
);


ALTER TABLE public.purchase_request OWNER TO postgres;

--
-- TOC entry 5215 (class 0 OID 0)
-- Dependencies: 331
-- Name: COLUMN purchase_request.warehouse_code; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.purchase_request.warehouse_code IS 'คลังสินค้า (optional) — FK to warehouse.warehouse_code, used to deduct stock on PR submit';


--
-- TOC entry 5216 (class 0 OID 0)
-- Dependencies: 331
-- Name: COLUMN purchase_request.memo_id; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.purchase_request.memo_id IS 'อ้างอิง Memo ต้นทาง (nullable) — ดูว่า PR นี้มาจากใบบันทึกไหน';


--
-- TOC entry 5217 (class 0 OID 0)
-- Dependencies: 331
-- Name: COLUMN purchase_request.location_text; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.purchase_request.location_text IS 'สถานที่ส่งของ — free text, no longer linked to location master';


--
-- TOC entry 332 (class 1259 OID 21292)
-- Name: purchase_request_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.purchase_request ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.purchase_request_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- TOC entry 333 (class 1259 OID 21293)
-- Name: purchase_request_line; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.purchase_request_line (
    id bigint NOT NULL,
    pr_id bigint NOT NULL,
    line_no integer NOT NULL,
    mat_code character varying(20) NOT NULL,
    qty_requested numeric(18,4) NOT NULL,
    qty_reserved numeric(18,4) DEFAULT 0 NOT NULL,
    qty_to_order numeric(18,4) DEFAULT 0 NOT NULL,
    remarks text,
    status character varying(30) DEFAULT 'OPEN'::character varying NOT NULL,
    qty_ordered numeric(18,4) DEFAULT 0 NOT NULL,
    cost_subgroup_id bigint,
    deduct_stock boolean DEFAULT true NOT NULL,
    memo_line_id bigint,
    CONSTRAINT pr_line_status_check CHECK (((status)::text = ANY (ARRAY['OPEN'::text, 'RESERVED'::text, 'ORDERED'::text, 'FULFILLED'::text, 'CANCELLED'::text]))),
    CONSTRAINT purchase_request_line_qty_requested_check CHECK ((qty_requested > (0)::numeric))
);


ALTER TABLE public.purchase_request_line OWNER TO postgres;

--
-- TOC entry 334 (class 1259 OID 21305)
-- Name: purchase_request_line_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.purchase_request_line ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.purchase_request_line_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- TOC entry 335 (class 1259 OID 21306)
-- Name: requisition; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.requisition (
    id bigint NOT NULL,
    req_no character varying(30) NOT NULL,
    project_code character varying(20) NOT NULL,
    warehouse_code character varying(20) NOT NULL,
    requester_id bigint NOT NULL,
    req_date date DEFAULT CURRENT_DATE NOT NULL,
    purpose text,
    status character varying(20) DEFAULT 'DRAFT'::character varying NOT NULL,
    checked_by bigint,
    checked_at timestamp without time zone,
    remarks text,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint NOT NULL,
    updated_by bigint,
    is_stock_house boolean DEFAULT false NOT NULL,
    container_no character varying(100),
    CONSTRAINT chk_requisition_container_no CHECK (((is_stock_house = false) OR (container_no IS NOT NULL))),
    CONSTRAINT requisition_status_check CHECK (((status)::text = ANY (ARRAY[('DRAFT'::character varying)::text, ('SUBMITTED'::character varying)::text, ('ISSUED'::character varying)::text, ('CANCELLED'::character varying)::text])))
);


ALTER TABLE public.requisition OWNER TO postgres;

--
-- TOC entry 336 (class 1259 OID 21316)
-- Name: requisition_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.requisition_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.requisition_id_seq OWNER TO postgres;

--
-- TOC entry 5218 (class 0 OID 0)
-- Dependencies: 336
-- Name: requisition_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.requisition_id_seq OWNED BY public.requisition.id;


--
-- TOC entry 337 (class 1259 OID 21317)
-- Name: requisition_line; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.requisition_line (
    id bigint NOT NULL,
    req_id bigint NOT NULL,
    line_no integer,
    stock_item_id bigint,
    mat_code character varying(20) NOT NULL,
    unit character varying(50),
    qty_requested numeric(18,4) NOT NULL,
    qty_issued numeric(18,4) DEFAULT 0 NOT NULL,
    unit_cost numeric(18,4),
    total_cost numeric(18,4),
    remarks text
);


ALTER TABLE public.requisition_line OWNER TO postgres;

--
-- TOC entry 338 (class 1259 OID 21323)
-- Name: requisition_line_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.requisition_line_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.requisition_line_id_seq OWNER TO postgres;

--
-- TOC entry 5219 (class 0 OID 0)
-- Dependencies: 338
-- Name: requisition_line_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.requisition_line_id_seq OWNED BY public.requisition_line.id;


--
-- TOC entry 339 (class 1259 OID 21324)
-- Name: requisition_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.requisition_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.requisition_seq OWNER TO postgres;

--
-- TOC entry 340 (class 1259 OID 21325)
-- Name: requisition_status_log; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.requisition_status_log (
    id bigint NOT NULL,
    req_id bigint NOT NULL,
    from_status character varying(30),
    to_status character varying(30) NOT NULL,
    changed_by bigint NOT NULL,
    changed_at timestamp without time zone DEFAULT now() NOT NULL,
    remarks text
);


ALTER TABLE public.requisition_status_log OWNER TO postgres;

--
-- TOC entry 341 (class 1259 OID 21331)
-- Name: requisition_status_log_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.requisition_status_log_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.requisition_status_log_id_seq OWNER TO postgres;

--
-- TOC entry 5220 (class 0 OID 0)
-- Dependencies: 341
-- Name: requisition_status_log_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.requisition_status_log_id_seq OWNED BY public.requisition_status_log.id;


--
-- TOC entry 420 (class 1259 OID 23727)
-- Name: ret_number_counter; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.ret_number_counter (
    year_month character varying NOT NULL,
    last_seq integer DEFAULT 0 NOT NULL
);


ALTER TABLE public.ret_number_counter OWNER TO postgres;

--
-- TOC entry 342 (class 1259 OID 21332)
-- Name: rfq; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.rfq (
    id bigint NOT NULL,
    rfq_no character varying(30) NOT NULL,
    rfq_date date DEFAULT CURRENT_DATE NOT NULL,
    pr_id bigint,
    status character varying(20) DEFAULT 'SENT'::character varying NOT NULL,
    remarks text,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint NOT NULL,
    updated_by bigint,
    supplier_id integer,
    CONSTRAINT rfq_status_check CHECK (((status)::text = ANY (ARRAY['SENT'::text, 'RECEIVED'::text, 'SELECTED'::text, 'REJECTED'::text])))
);


ALTER TABLE public.rfq OWNER TO postgres;

--
-- TOC entry 343 (class 1259 OID 21342)
-- Name: rfq_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.rfq ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.rfq_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- TOC entry 344 (class 1259 OID 21343)
-- Name: rfq_line; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.rfq_line (
    id bigint NOT NULL,
    rfq_id bigint NOT NULL,
    line_no integer NOT NULL,
    mat_code character varying(20) NOT NULL,
    qty numeric(18,4) NOT NULL,
    unit_price numeric(18,4),
    currency character varying(10) DEFAULT 'THB'::character varying,
    lead_time_days integer,
    remarks text
);


ALTER TABLE public.rfq_line OWNER TO postgres;

--
-- TOC entry 345 (class 1259 OID 21349)
-- Name: rfq_line_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.rfq_line ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.rfq_line_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- TOC entry 346 (class 1259 OID 21350)
-- Name: role_menu_permissions; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.role_menu_permissions (
    id bigint NOT NULL,
    role_id bigint NOT NULL,
    menu_id bigint NOT NULL,
    can_read boolean DEFAULT false NOT NULL,
    can_write boolean DEFAULT false NOT NULL,
    can_update boolean DEFAULT false NOT NULL,
    can_delete boolean DEFAULT false NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint
);


ALTER TABLE public.role_menu_permissions OWNER TO postgres;

--
-- TOC entry 347 (class 1259 OID 21359)
-- Name: role_menu_permissions_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.role_menu_permissions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.role_menu_permissions_id_seq OWNER TO postgres;

--
-- TOC entry 5221 (class 0 OID 0)
-- Dependencies: 347
-- Name: role_menu_permissions_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.role_menu_permissions_id_seq OWNED BY public.role_menu_permissions.id;


--
-- TOC entry 348 (class 1259 OID 21360)
-- Name: role_menus; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.role_menus (
    role_id bigint NOT NULL,
    menu_id bigint NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint
);


ALTER TABLE public.role_menus OWNER TO postgres;

--
-- TOC entry 349 (class 1259 OID 21364)
-- Name: role_permissions; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.role_permissions (
    role_id bigint NOT NULL,
    permission_id bigint NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint
);


ALTER TABLE public.role_permissions OWNER TO postgres;

--
-- TOC entry 350 (class 1259 OID 21368)
-- Name: roles; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.roles (
    id bigint NOT NULL,
    role_code character varying(100) NOT NULL,
    role_name character varying(255) NOT NULL,
    description text,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint,
    is_active boolean DEFAULT true NOT NULL,
    level integer DEFAULT 0 NOT NULL,
    department character varying(100),
    dept_code character varying(50)
);


ALTER TABLE public.roles OWNER TO postgres;

--
-- TOC entry 351 (class 1259 OID 21377)
-- Name: roles_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.roles ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.roles_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- TOC entry 352 (class 1259 OID 21378)
-- Name: spec_size; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.spec_size (
    id integer NOT NULL,
    spec_code character varying(10) NOT NULL,
    mat_name_id integer NOT NULL,
    spec_description character varying(200) NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint
);


ALTER TABLE public.spec_size OWNER TO postgres;

--
-- TOC entry 353 (class 1259 OID 21384)
-- Name: spec_size_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.spec_size_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.spec_size_id_seq OWNER TO postgres;

--
-- TOC entry 5222 (class 0 OID 0)
-- Dependencies: 353
-- Name: spec_size_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.spec_size_id_seq OWNED BY public.spec_size.id;


--
-- TOC entry 354 (class 1259 OID 21385)
-- Name: stock_category; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.stock_category (
    id bigint NOT NULL,
    code character varying(20) NOT NULL,
    name character varying(100) NOT NULL,
    description text,
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL
);


ALTER TABLE public.stock_category OWNER TO postgres;

--
-- TOC entry 5223 (class 0 OID 0)
-- Dependencies: 354
-- Name: TABLE stock_category; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON TABLE public.stock_category IS 'หมวดหมู่ stock item — แยกจาก mat_group ของ PR/PO';


--
-- TOC entry 355 (class 1259 OID 21392)
-- Name: stock_category_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.stock_category ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.stock_category_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- TOC entry 356 (class 1259 OID 21393)
-- Name: stock_count; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.stock_count (
    id bigint NOT NULL,
    count_no character varying(30) NOT NULL,
    warehouse_code character varying(20) NOT NULL,
    count_date date NOT NULL,
    status character varying(20) DEFAULT 'DRAFT'::character varying NOT NULL,
    remarks text,
    approved_by bigint,
    approved_at timestamp without time zone,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint NOT NULL,
    updated_by bigint,
    CONSTRAINT stock_count_status_check CHECK (((status)::text = ANY (ARRAY['DRAFT'::text, 'IN_PROGRESS'::text, 'COMPLETED'::text])))
);


ALTER TABLE public.stock_count OWNER TO postgres;

--
-- TOC entry 357 (class 1259 OID 21402)
-- Name: stock_count_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.stock_count ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.stock_count_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- TOC entry 358 (class 1259 OID 21403)
-- Name: stock_count_line; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.stock_count_line (
    id bigint NOT NULL,
    count_id bigint NOT NULL,
    mat_code character varying(20) NOT NULL,
    zone_id integer,
    qty_system numeric(18,4),
    qty_counted numeric(18,4),
    qty_diff numeric(18,4) GENERATED ALWAYS AS ((qty_counted - qty_system)) STORED,
    remarks text
);


ALTER TABLE public.stock_count_line OWNER TO postgres;

--
-- TOC entry 359 (class 1259 OID 21409)
-- Name: stock_count_line_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.stock_count_line ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.stock_count_line_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- TOC entry 360 (class 1259 OID 21410)
-- Name: stock_inventory; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.stock_inventory (
    id bigint NOT NULL,
    item_id bigint NOT NULL,
    location_code character varying(20) NOT NULL,
    warehouse_code character varying(20) NOT NULL,
    qty_on_hand numeric(18,4) DEFAULT 0 NOT NULL,
    qty_reserved numeric(18,4) DEFAULT 0 NOT NULL,
    qty_available numeric(18,4) GENERATED ALWAYS AS ((qty_on_hand - qty_reserved)) STORED,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    CONSTRAINT stock_inventory_qty_check CHECK ((qty_on_hand >= (0)::numeric)),
    CONSTRAINT stock_inventory_res_check CHECK ((qty_reserved >= (0)::numeric))
);


ALTER TABLE public.stock_inventory OWNER TO postgres;

--
-- TOC entry 5224 (class 0 OID 0)
-- Dependencies: 360
-- Name: TABLE stock_inventory; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON TABLE public.stock_inventory IS 'Stock balance per item per location';


--
-- TOC entry 5225 (class 0 OID 0)
-- Dependencies: 360
-- Name: COLUMN stock_inventory.qty_available; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.stock_inventory.qty_available IS 'qty_on_hand - qty_reserved (generated)';


--
-- TOC entry 361 (class 1259 OID 21419)
-- Name: stock_inventory_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.stock_inventory ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.stock_inventory_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- TOC entry 362 (class 1259 OID 21420)
-- Name: stock_item; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.stock_item (
    id bigint NOT NULL,
    mat_code character varying(30) NOT NULL,
    item_name character varying(255) NOT NULL,
    description text,
    category_id bigint,
    item_type character varying(20) DEFAULT 'RETURNABLE'::character varying NOT NULL,
    tracking_type character varying(10) DEFAULT 'sku'::character varying NOT NULL,
    unit character varying(50) NOT NULL,
    qty numeric(18,4) DEFAULT 0 NOT NULL,
    qr_code text,
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint,
    unit_cost numeric(18,4) DEFAULT 0 NOT NULL,
    warehouse_code character varying(20) DEFAULT 'WH01'::character varying NOT NULL,
    location_code character varying(20) DEFAULT 'SAL'::character varying NOT NULL,
    description_store text,
    CONSTRAINT stock_item_min_qty_check CHECK ((qty >= (0)::numeric)),
    CONSTRAINT stock_item_tracking_check CHECK (((tracking_type)::text = ANY (ARRAY['sku'::text, 'serial'::text]))),
    CONSTRAINT stock_item_type_check CHECK (((item_type)::text = ANY (ARRAY['RETURNABLE'::text, 'CONSUMABLE'::text]))),
    CONSTRAINT stock_item_unit_cost_check CHECK ((unit_cost >= (0)::numeric))
);


ALTER TABLE public.stock_item OWNER TO postgres;

--
-- TOC entry 5226 (class 0 OID 0)
-- Dependencies: 362
-- Name: TABLE stock_item; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON TABLE public.stock_item IS 'Stock item master — คนละตัวกับ material_code ของ PR/PO';


--
-- TOC entry 5227 (class 0 OID 0)
-- Dependencies: 362
-- Name: COLUMN stock_item.mat_code; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.stock_item.mat_code IS 'ใช้ค่าเดียวกับ mat_code ฝั่ง Material master (PR/PO) แต่เป็นคนละตาราง
   ไม่มี FK ข้าม schema — ระวังตอน query/join ว่ากำลังอ้างถึง mat_code
   ฝั่ง Stock หรือฝั่ง Material';


--
-- TOC entry 5228 (class 0 OID 0)
-- Dependencies: 362
-- Name: COLUMN stock_item.item_type; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.stock_item.item_type IS 'RETURNABLE=ต้องคืน, CONSUMABLE=ไม่ต้องคืน';


--
-- TOC entry 5229 (class 0 OID 0)
-- Dependencies: 362
-- Name: COLUMN stock_item.tracking_type; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.stock_item.tracking_type IS 'sku=per SKU (ปัจจุบัน), serial=per unit (อนาคต)';


--
-- TOC entry 5230 (class 0 OID 0)
-- Dependencies: 362
-- Name: COLUMN stock_item.qty; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.stock_item.qty IS 'จำนวนคงเหลือ (qty on hand) — เก็บตรงบน stock_item ไปก่อน ยังไม่แยกตาม
   location เต็มรูปแบบ ดู warehouse_code/location_code คู่กันสำหรับรู้ว่า
   อยู่คลังไหน จะเชื่อมกับระบบ multi-location stock_inventory ทีหลัง';


--
-- TOC entry 5231 (class 0 OID 0)
-- Dependencies: 362
-- Name: COLUMN stock_item.qr_code; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.stock_item.qr_code IS 'QR code value — default = item_code';


--
-- TOC entry 5232 (class 0 OID 0)
-- Dependencies: 362
-- Name: COLUMN stock_item.unit_cost; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.stock_item.unit_cost IS 'ต้นทุนต่อหน่วย (บาท) — จาก column "UNIT" ใต้หัว "MATERIAL COST" ในไฟล์
   Excel ต้นทาง ใช้คำนวณ amount = qty * unit_cost ตอนแสดงผล ไม่ต้อง
   เก็บ amount แยกในตารางนี้';


--
-- TOC entry 363 (class 1259 OID 21438)
-- Name: stock_item_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.stock_item ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.stock_item_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- TOC entry 364 (class 1259 OID 21439)
-- Name: stock_item_image; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.stock_item_image (
    id bigint NOT NULL,
    item_id bigint NOT NULL,
    file_path text NOT NULL,
    file_name character varying(255),
    is_primary boolean DEFAULT false NOT NULL,
    sort_order integer DEFAULT 0 NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint
);


ALTER TABLE public.stock_item_image OWNER TO postgres;

--
-- TOC entry 365 (class 1259 OID 21447)
-- Name: stock_item_image_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.stock_item_image_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.stock_item_image_id_seq OWNER TO postgres;

--
-- TOC entry 5233 (class 0 OID 0)
-- Dependencies: 365
-- Name: stock_item_image_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.stock_item_image_id_seq OWNED BY public.stock_item_image.id;


--
-- TOC entry 424 (class 1259 OID 24435)
-- Name: stock_item_qty_backup_20261011; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.stock_item_qty_backup_20261011 (
    id bigint,
    mat_code character varying(30),
    item_name character varying(255),
    description text,
    category_id bigint,
    item_type character varying(20),
    tracking_type character varying(10),
    unit character varying(50),
    qty numeric(18,4),
    qr_code text,
    is_active boolean,
    created_at timestamp without time zone,
    updated_at timestamp without time zone,
    created_by bigint,
    updated_by bigint,
    unit_cost numeric(18,4),
    warehouse_code character varying(20),
    location_code character varying(20),
    description_store text
);


ALTER TABLE public.stock_item_qty_backup_20261011 OWNER TO postgres;

--
-- TOC entry 366 (class 1259 OID 21448)
-- Name: stock_item_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.stock_item_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.stock_item_seq OWNER TO postgres;

--
-- TOC entry 367 (class 1259 OID 21449)
-- Name: stock_res_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.stock_res_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.stock_res_seq OWNER TO postgres;

--
-- TOC entry 368 (class 1259 OID 21450)
-- Name: stock_reservation; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.stock_reservation (
    id bigint NOT NULL,
    reservation_no character varying(30) NOT NULL,
    item_id bigint NOT NULL,
    location_code character varying(20) NOT NULL,
    qty_reserved numeric(18,4) NOT NULL,
    qty_fulfilled numeric(18,4) DEFAULT 0 NOT NULL,
    status character varying(20) DEFAULT 'PENDING'::character varying NOT NULL,
    requested_by bigint NOT NULL,
    needed_by date,
    purpose text,
    ref_doc_type character varying(20),
    ref_doc_id bigint,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    cancelled_at timestamp without time zone,
    CONSTRAINT stock_res_qty_check CHECK ((qty_reserved > (0)::numeric)),
    CONSTRAINT stock_res_status_check CHECK (((status)::text = ANY (ARRAY['PENDING'::text, 'CONFIRMED'::text, 'CANCELLED'::text, 'FULFILLED'::text])))
);


ALTER TABLE public.stock_reservation OWNER TO postgres;

--
-- TOC entry 5234 (class 0 OID 0)
-- Dependencies: 368
-- Name: TABLE stock_reservation; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON TABLE public.stock_reservation IS 'จองวัสดุล่วงหน้า ป้องกัน over-commit';


--
-- TOC entry 369 (class 1259 OID 21461)
-- Name: stock_reservation_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.stock_reservation ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.stock_reservation_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- TOC entry 370 (class 1259 OID 21462)
-- Name: stock_transaction; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.stock_transaction (
    id bigint NOT NULL,
    txn_no character varying(30) NOT NULL,
    txn_type character varying(20) NOT NULL,
    item_id bigint NOT NULL,
    from_location character varying(20),
    to_location character varying(20),
    qty numeric(18,4) NOT NULL,
    ref_doc_type character varying(20),
    ref_doc_id bigint,
    remarks text,
    txn_date date DEFAULT CURRENT_DATE NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint NOT NULL,
    qty_before numeric(18,4),
    qty_after numeric(18,4),
    reversed_at timestamp without time zone,
    receive_document_id bigint,
    return_document_id bigint,
    CONSTRAINT stock_txn_qty_check CHECK ((qty > (0)::numeric)),
    CONSTRAINT stock_txn_type_check CHECK (((txn_type)::text = ANY (ARRAY['IN'::text, 'OUT'::text, 'TRANSFER'::text, 'ADJUST_PLUS'::text, 'ADJUST_MINUS'::text, 'BORROW_OUT'::text, 'BORROW_RETURN'::text])))
);


ALTER TABLE public.stock_transaction OWNER TO postgres;

--
-- TOC entry 5235 (class 0 OID 0)
-- Dependencies: 370
-- Name: COLUMN stock_transaction.qty_before; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.stock_transaction.qty_before IS 'ยอดคงเหลือก่อนรายการนี้ (snapshot ตอน insert เท่านั้น ไม่ update ย้อนหลัง)';


--
-- TOC entry 5236 (class 0 OID 0)
-- Dependencies: 370
-- Name: COLUMN stock_transaction.qty_after; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.stock_transaction.qty_after IS 'ยอดคงเหลือหลังรายการนี้';


--
-- TOC entry 371 (class 1259 OID 21471)
-- Name: stock_transaction_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.stock_transaction ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.stock_transaction_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- TOC entry 372 (class 1259 OID 21472)
-- Name: stock_transfer; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.stock_transfer (
    id bigint NOT NULL,
    transfer_no character varying(30) NOT NULL,
    transfer_type character varying(20) NOT NULL,
    transfer_date date NOT NULL,
    from_warehouse_code character varying(20),
    from_project_code character varying(20),
    to_warehouse_code character varying(20),
    to_project_code character varying(20),
    requested_by bigint NOT NULL,
    purpose text,
    remarks text,
    status character varying(20) DEFAULT 'DRAFT'::character varying NOT NULL,
    checked_by bigint,
    checked_at timestamp without time zone,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint NOT NULL,
    updated_by bigint
);


ALTER TABLE public.stock_transfer OWNER TO postgres;

--
-- TOC entry 373 (class 1259 OID 21481)
-- Name: stock_transfer_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.stock_transfer ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.stock_transfer_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- TOC entry 374 (class 1259 OID 21482)
-- Name: stock_transfer_line; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.stock_transfer_line (
    id bigint NOT NULL,
    transfer_id bigint NOT NULL,
    line_no integer NOT NULL,
    item_id bigint NOT NULL,
    mat_code character varying(30) NOT NULL,
    unit character varying(50),
    qty_requested numeric NOT NULL,
    qty_confirmed numeric,
    remarks text
);


ALTER TABLE public.stock_transfer_line OWNER TO postgres;

--
-- TOC entry 375 (class 1259 OID 21487)
-- Name: stock_transfer_line_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.stock_transfer_line ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.stock_transfer_line_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- TOC entry 376 (class 1259 OID 21488)
-- Name: stock_transfer_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.stock_transfer_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.stock_transfer_seq OWNER TO postgres;

--
-- TOC entry 377 (class 1259 OID 21489)
-- Name: stock_transfer_status_log; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.stock_transfer_status_log (
    id bigint NOT NULL,
    transfer_id bigint NOT NULL,
    from_status character varying(20),
    to_status character varying(20) NOT NULL,
    changed_by bigint NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL
);


ALTER TABLE public.stock_transfer_status_log OWNER TO postgres;

--
-- TOC entry 378 (class 1259 OID 21495)
-- Name: stock_transfer_status_log_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.stock_transfer_status_log ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.stock_transfer_status_log_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- TOC entry 379 (class 1259 OID 21496)
-- Name: stock_txn_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.stock_txn_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.stock_txn_seq OWNER TO postgres;

--
-- TOC entry 380 (class 1259 OID 21497)
-- Name: storage_zone; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.storage_zone (
    id integer NOT NULL,
    warehouse_id integer NOT NULL,
    zone_code character varying(30) NOT NULL,
    zone_name character varying(100) NOT NULL,
    zone_type character varying(50),
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint,
    CONSTRAINT storage_zone_type_check CHECK (((zone_type)::text = ANY (ARRAY['rack'::text, 'shelf'::text, 'bin'::text, 'area'::text])))
);


ALTER TABLE public.storage_zone OWNER TO postgres;

--
-- TOC entry 381 (class 1259 OID 21503)
-- Name: storage_zone_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.storage_zone_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.storage_zone_id_seq OWNER TO postgres;

--
-- TOC entry 5237 (class 0 OID 0)
-- Dependencies: 381
-- Name: storage_zone_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.storage_zone_id_seq OWNED BY public.storage_zone.id;


--
-- TOC entry 382 (class 1259 OID 21504)
-- Name: subgroup; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.subgroup (
    id integer NOT NULL,
    subgroup_code character varying(10) NOT NULL,
    group_id integer NOT NULL,
    subgroup_name character varying(100) NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint
);


ALTER TABLE public.subgroup OWNER TO postgres;

--
-- TOC entry 383 (class 1259 OID 21510)
-- Name: subgroup_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.subgroup_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.subgroup_id_seq OWNER TO postgres;

--
-- TOC entry 5238 (class 0 OID 0)
-- Dependencies: 383
-- Name: subgroup_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.subgroup_id_seq OWNED BY public.subgroup.id;


--
-- TOC entry 384 (class 1259 OID 21511)
-- Name: supplier; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.supplier (
    id integer NOT NULL,
    supplier_name character varying(200) NOT NULL,
    tax_id character varying(50),
    address text,
    contact_name character varying(200),
    contact_phone character varying(50),
    contact_email character varying(200),
    payment_terms character varying(100),
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint,
    office_phone character varying(50),
    fax character varying(50),
    currency character varying(10) DEFAULT 'BH'::character varying,
    sales_person character varying,
    sales_person_phone character varying(50),
    remarks text,
    CONSTRAINT supplier_currency_check CHECK (((currency)::text = ANY (ARRAY[('BH'::character varying)::text, ('US'::character varying)::text])))
);


ALTER TABLE public.supplier OWNER TO postgres;

--
-- TOC entry 5239 (class 0 OID 0)
-- Dependencies: 384
-- Name: COLUMN supplier.office_phone; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.supplier.office_phone IS 'เบอร์โทรศัพท์สำนักงาน';


--
-- TOC entry 5240 (class 0 OID 0)
-- Dependencies: 384
-- Name: COLUMN supplier.fax; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.supplier.fax IS 'หมายเลขแฟกซ์';


--
-- TOC entry 5241 (class 0 OID 0)
-- Dependencies: 384
-- Name: COLUMN supplier.currency; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.supplier.currency IS 'สกุลเงิน: BH = บาท, US = ดอลลาร์';


--
-- TOC entry 385 (class 1259 OID 21521)
-- Name: supplier_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.supplier_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.supplier_id_seq OWNER TO postgres;

--
-- TOC entry 5242 (class 0 OID 0)
-- Dependencies: 385
-- Name: supplier_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.supplier_id_seq OWNED BY public.supplier.id;


--
-- TOC entry 386 (class 1259 OID 21522)
-- Name: unit; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.unit (
    id integer NOT NULL,
    unit_code character varying(10) NOT NULL,
    unit_name character varying(50) NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint
);


ALTER TABLE public.unit OWNER TO postgres;

--
-- TOC entry 387 (class 1259 OID 21528)
-- Name: unit_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.unit_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.unit_id_seq OWNER TO postgres;

--
-- TOC entry 5243 (class 0 OID 0)
-- Dependencies: 387
-- Name: unit_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.unit_id_seq OWNED BY public.unit.id;


--
-- TOC entry 388 (class 1259 OID 21529)
-- Name: user_menu_permissions; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.user_menu_permissions (
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    menu_id bigint NOT NULL,
    can_read boolean DEFAULT false NOT NULL,
    can_write boolean DEFAULT false NOT NULL,
    can_update boolean DEFAULT false NOT NULL,
    can_delete boolean DEFAULT false NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint
);


ALTER TABLE public.user_menu_permissions OWNER TO postgres;

--
-- TOC entry 389 (class 1259 OID 21538)
-- Name: user_menu_permissions_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.user_menu_permissions ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.user_menu_permissions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- TOC entry 390 (class 1259 OID 21539)
-- Name: user_permissions; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.user_permissions (
    user_id bigint NOT NULL,
    permission_id bigint NOT NULL,
    is_allow boolean NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint
);


ALTER TABLE public.user_permissions OWNER TO postgres;

--
-- TOC entry 391 (class 1259 OID 21544)
-- Name: user_roles; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.user_roles (
    user_id bigint NOT NULL,
    role_id bigint NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint
);


ALTER TABLE public.user_roles OWNER TO postgres;

--
-- TOC entry 392 (class 1259 OID 21548)
-- Name: users; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.users (
    id bigint NOT NULL,
    username character varying(100) NOT NULL,
    full_name character varying(255) NOT NULL,
    email character varying(255),
    password_hash text,
    is_active boolean DEFAULT true NOT NULL,
    location_code character varying(20),
    employee_code character varying(50),
    department character varying(200),
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint,
    dept_code character varying(50),
    signature_path text,
    signature_mime character varying(50),
    signature_updated_at timestamp without time zone,
    full_name_th character varying(255),
    phone character varying(30)
);


ALTER TABLE public.users OWNER TO postgres;

--
-- TOC entry 5244 (class 0 OID 0)
-- Dependencies: 392
-- Name: COLUMN users.full_name_th; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.users.full_name_th IS 'ชื่อ-นามสกุลภาษาไทย (optional); full_name เดิมเก็บชื่อภาษาอังกฤษ';


--
-- TOC entry 393 (class 1259 OID 21556)
-- Name: users_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.users ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.users_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- TOC entry 394 (class 1259 OID 21557)
-- Name: warehouse; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.warehouse (
    id integer NOT NULL,
    warehouse_code character varying(20) NOT NULL,
    warehouse_name character varying(200) NOT NULL,
    address text,
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint
);


ALTER TABLE public.warehouse OWNER TO postgres;

--
-- TOC entry 395 (class 1259 OID 21565)
-- Name: warehouse_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.warehouse_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.warehouse_id_seq OWNER TO postgres;

--
-- TOC entry 5245 (class 0 OID 0)
-- Dependencies: 395
-- Name: warehouse_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.warehouse_id_seq OWNED BY public.warehouse.id;


--
-- TOC entry 423 (class 1259 OID 23744)
-- Name: wht_income_type; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.wht_income_type (
    id integer NOT NULL,
    code character varying(30) NOT NULL,
    label_th character varying(150) NOT NULL,
    rate numeric(5,2) NOT NULL,
    sort_order integer DEFAULT 0 NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT wht_income_type_rate_check CHECK (((rate > (0)::numeric) AND (rate <= (15)::numeric)))
);


ALTER TABLE public.wht_income_type OWNER TO postgres;

--
-- TOC entry 422 (class 1259 OID 23743)
-- Name: wht_income_type_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.wht_income_type ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.wht_income_type_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- TOC entry 396 (class 1259 OID 21566)
-- Name: wo_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.wo_seq
    START WITH 2
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.wo_seq OWNER TO postgres;

--
-- TOC entry 397 (class 1259 OID 21567)
-- Name: work_order; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.work_order (
    id bigint NOT NULL,
    wo_no character varying(30) NOT NULL,
    wo_date date DEFAULT CURRENT_DATE NOT NULL,
    employer_name character varying(200) NOT NULL,
    project_code character varying(20),
    project_scope_text text,
    supplier_name character varying(200) NOT NULL,
    contact_person character varying(100),
    supplier_address text,
    supplier_phone character varying(50),
    contract_type character varying(20) DEFAULT 'LABOR_MATERIAL'::character varying NOT NULL,
    work_system text[] NOT NULL,
    contract_description text,
    contract_amount numeric(18,2) DEFAULT 0 NOT NULL,
    vat_rate numeric(5,2) DEFAULT 7.00 NOT NULL,
    wht_rate numeric(5,2) DEFAULT 3.00 NOT NULL,
    advance_pct numeric(5,2) DEFAULT 0 NOT NULL,
    advance_amount numeric(18,2) DEFAULT 0 NOT NULL,
    progress_payment_note text,
    retention_pct numeric(5,2) DEFAULT 5.00 NOT NULL,
    advance_deduct_pct numeric(5,2) DEFAULT 0 NOT NULL,
    other_deduction_note text,
    start_date date,
    duration_days integer,
    end_date date,
    penalty_pct_per_day numeric(5,2) DEFAULT 0 NOT NULL,
    warranty_years integer DEFAULT 1 NOT NULL,
    ref_no character varying(50),
    other_terms text,
    cost_code character varying(50),
    status character varying(20) DEFAULT 'DRAFT'::character varying NOT NULL,
    entered_by bigint,
    entered_at timestamp without time zone,
    section_head_id bigint,
    section_head_signed_at timestamp without time zone,
    authorized_by bigint,
    authorized_at timestamp without time zone,
    subcontractor_signed_name character varying(200),
    subcontractor_signed_at timestamp without time zone,
    remarks text,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint NOT NULL,
    updated_by bigint,
    use_discount boolean DEFAULT false NOT NULL,
    discount_type character varying(10) DEFAULT 'pct'::character varying NOT NULL,
    use_vat boolean DEFAULT false NOT NULL,
    use_wht boolean DEFAULT false NOT NULL,
    total_amount numeric(18,4) DEFAULT 0 NOT NULL,
    discount_amount numeric(18,4) DEFAULT 0 NOT NULL,
    vat_amount numeric(18,4) DEFAULT 0 NOT NULL,
    wht_amount numeric(18,4) DEFAULT 0 NOT NULL,
    net_amount numeric(18,4) DEFAULT 0 NOT NULL,
    supplier_id integer,
    project_name_text character varying(200),
    advance_deduct_amount numeric(18,2) DEFAULT 0 NOT NULL,
    approver_id bigint,
    CONSTRAINT chk_wo_contract_type CHECK (((contract_type)::text = ANY (ARRAY[('LABOR_MATERIAL'::character varying)::text, ('LABOR_ONLY'::character varying)::text, ('RENTAL'::character varying)::text, ('SERVICE'::character varying)::text]))),
    CONSTRAINT chk_wo_status CHECK (((status)::text = ANY (ARRAY['DRAFT'::text, 'PENDING_APPROVAL'::text, 'APPROVED'::text, 'REJECTED'::text, 'PENDING_REAPPROVAL'::text, 'CANCELLED'::text]))),
    CONSTRAINT chk_wo_work_system CHECK (((cardinality(work_system) >= 1) AND (work_system <@ ARRAY['P'::text, 'E'::text, 'S'::text, 'MP'::text, 'ME'::text, 'MS'::text, 'MF'::text, 'MG'::text, 'MH'::text]))),
    CONSTRAINT wo_discount_type_check CHECK (((discount_type)::text = ANY (ARRAY[('pct'::character varying)::text, ('amt'::character varying)::text])))
);


ALTER TABLE public.work_order OWNER TO postgres;

--
-- TOC entry 398 (class 1259 OID 21598)
-- Name: work_order_attachment; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.work_order_attachment (
    id bigint NOT NULL,
    wo_id bigint NOT NULL,
    file_name character varying(255) NOT NULL,
    file_path character varying(500) NOT NULL,
    file_size bigint,
    file_type character varying(50),
    uploaded_at timestamp without time zone DEFAULT now() NOT NULL,
    uploaded_by bigint NOT NULL
);


ALTER TABLE public.work_order_attachment OWNER TO postgres;

--
-- TOC entry 399 (class 1259 OID 21604)
-- Name: work_order_attachment_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.work_order_attachment ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.work_order_attachment_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- TOC entry 400 (class 1259 OID 21605)
-- Name: work_order_cost_code; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.work_order_cost_code (
    id bigint NOT NULL,
    wo_id bigint NOT NULL,
    cost_code character varying(50) NOT NULL,
    remarks character varying(255),
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint NOT NULL
);


ALTER TABLE public.work_order_cost_code OWNER TO postgres;

--
-- TOC entry 401 (class 1259 OID 21609)
-- Name: work_order_cost_code_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.work_order_cost_code ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.work_order_cost_code_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- TOC entry 434 (class 1259 OID 26454)
-- Name: work_order_edit_log; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.work_order_edit_log (
    id bigint NOT NULL,
    wo_id bigint NOT NULL,
    edited_by bigint NOT NULL,
    reason text NOT NULL,
    edited_at timestamp without time zone DEFAULT now() NOT NULL
);


ALTER TABLE public.work_order_edit_log OWNER TO postgres;

--
-- TOC entry 433 (class 1259 OID 26453)
-- Name: work_order_edit_log_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.work_order_edit_log ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.work_order_edit_log_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- TOC entry 402 (class 1259 OID 21610)
-- Name: work_order_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.work_order ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.work_order_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- TOC entry 403 (class 1259 OID 21611)
-- Name: work_order_line; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.work_order_line (
    id bigint NOT NULL,
    wo_id bigint NOT NULL,
    sort_order integer NOT NULL,
    cost_code character varying(50) NOT NULL,
    description text,
    qty numeric(18,4) NOT NULL,
    unit_price numeric(18,4) NOT NULL,
    amount numeric(18,4) GENERATED ALWAYS AS ((qty * unit_price)) STORED,
    disc numeric(18,4) DEFAULT 0 NOT NULL,
    disc_type character varying(10) DEFAULT 'pct'::character varying NOT NULL,
    vat_rate numeric(5,2),
    wht_rate numeric(5,2),
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint NOT NULL,
    CONSTRAINT chk_wol_wht_rate CHECK (((wht_rate IS NULL) OR (wht_rate = ANY (ARRAY[(1)::numeric, (3)::numeric, (5)::numeric])))),
    CONSTRAINT work_order_line_disc_type_check CHECK (((disc_type)::text = ANY (ARRAY[('pct'::character varying)::text, ('amt'::character varying)::text]))),
    CONSTRAINT work_order_line_qty_check CHECK ((qty > (0)::numeric)),
    CONSTRAINT work_order_line_unit_price_check CHECK ((unit_price >= (0)::numeric)),
    CONSTRAINT work_order_line_wht_rate_check CHECK (((wht_rate IS NULL) OR (wht_rate = ANY (ARRAY[(1)::numeric, (3)::numeric, (5)::numeric]))))
);


ALTER TABLE public.work_order_line OWNER TO postgres;

--
-- TOC entry 404 (class 1259 OID 21625)
-- Name: work_order_line_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.work_order_line ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.work_order_line_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- TOC entry 405 (class 1259 OID 21626)
-- Name: work_order_payment_installment; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.work_order_payment_installment (
    id bigint NOT NULL,
    wo_id bigint NOT NULL,
    installment_no integer NOT NULL,
    description text,
    percent_of_contract numeric(5,2),
    amount numeric(18,2) DEFAULT 0 NOT NULL,
    due_date date,
    payment_status character varying(20) DEFAULT 'UNPAID'::character varying NOT NULL,
    paid_date date,
    remarks text,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint
);


ALTER TABLE public.work_order_payment_installment OWNER TO postgres;

--
-- TOC entry 406 (class 1259 OID 21635)
-- Name: work_order_payment_installment_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.work_order_payment_installment_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.work_order_payment_installment_id_seq OWNER TO postgres;

--
-- TOC entry 5246 (class 0 OID 0)
-- Dependencies: 406
-- Name: work_order_payment_installment_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.work_order_payment_installment_id_seq OWNED BY public.work_order_payment_installment.id;


--
-- TOC entry 407 (class 1259 OID 21636)
-- Name: work_order_penalty; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.work_order_penalty (
    id bigint NOT NULL,
    wo_id bigint NOT NULL,
    description text,
    percent_per_day numeric(5,2),
    contract_start_date date,
    contract_end_date date,
    remarks text,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint
);


ALTER TABLE public.work_order_penalty OWNER TO postgres;

--
-- TOC entry 408 (class 1259 OID 21643)
-- Name: work_order_penalty_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.work_order_penalty_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.work_order_penalty_id_seq OWNER TO postgres;

--
-- TOC entry 5247 (class 0 OID 0)
-- Dependencies: 408
-- Name: work_order_penalty_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.work_order_penalty_id_seq OWNED BY public.work_order_penalty.id;


--
-- TOC entry 409 (class 1259 OID 21644)
-- Name: work_order_retention; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.work_order_retention (
    id bigint NOT NULL,
    wo_id bigint NOT NULL,
    description text,
    percent_of_contract numeric(5,2),
    amount numeric(18,2) DEFAULT 0 NOT NULL,
    remarks text,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint
);


ALTER TABLE public.work_order_retention OWNER TO postgres;

--
-- TOC entry 410 (class 1259 OID 21652)
-- Name: work_order_retention_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.work_order_retention_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.work_order_retention_id_seq OWNER TO postgres;

--
-- TOC entry 5248 (class 0 OID 0)
-- Dependencies: 410
-- Name: work_order_retention_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.work_order_retention_id_seq OWNED BY public.work_order_retention.id;


--
-- TOC entry 411 (class 1259 OID 21653)
-- Name: work_order_status_log; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.work_order_status_log (
    id bigint NOT NULL,
    wo_id bigint NOT NULL,
    from_status character varying(20),
    to_status character varying(20) NOT NULL,
    changed_by bigint NOT NULL,
    changed_at timestamp without time zone DEFAULT now() NOT NULL,
    remarks text
);


ALTER TABLE public.work_order_status_log OWNER TO postgres;

--
-- TOC entry 412 (class 1259 OID 21659)
-- Name: work_order_status_log_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.work_order_status_log ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.work_order_status_log_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- TOC entry 3826 (class 2604 OID 21660)
-- Name: brand id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.brand ALTER COLUMN id SET DEFAULT nextval('public.brand_id_seq'::regclass);


--
-- TOC entry 3830 (class 2604 OID 21661)
-- Name: cost_group id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.cost_group ALTER COLUMN id SET DEFAULT nextval('public.cost_group_id_seq'::regclass);


--
-- TOC entry 3834 (class 2604 OID 21662)
-- Name: cost_job id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.cost_job ALTER COLUMN id SET DEFAULT nextval('public.cost_job_id_seq'::regclass);


--
-- TOC entry 3838 (class 2604 OID 21663)
-- Name: cost_subgroup id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.cost_subgroup ALTER COLUMN id SET DEFAULT nextval('public.cost_subgroup_id_seq'::regclass);


--
-- TOC entry 3844 (class 2604 OID 21664)
-- Name: cost_subject id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.cost_subject ALTER COLUMN id SET DEFAULT nextval('public.cost_subject_id_seq'::regclass);


--
-- TOC entry 3848 (class 2604 OID 21665)
-- Name: customer cus_id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.customer ALTER COLUMN cus_id SET DEFAULT nextval('public.customer_cus_id_seq'::regclass);


--
-- TOC entry 3870 (class 2604 OID 21666)
-- Name: ic_po_receive_document id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.ic_po_receive_document ALTER COLUMN id SET DEFAULT nextval('public.ic_po_receive_document_id_seq'::regclass);


--
-- TOC entry 3873 (class 2604 OID 21667)
-- Name: ic_project_cost_item id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.ic_project_cost_item ALTER COLUMN id SET DEFAULT nextval('public.ic_project_cost_item_id_seq'::regclass);


--
-- TOC entry 3878 (class 2604 OID 21668)
-- Name: ic_project_cost_item_transaction id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.ic_project_cost_item_transaction ALTER COLUMN id SET DEFAULT nextval('public.ic_project_cost_item_transaction_id_seq'::regclass);


--
-- TOC entry 3900 (class 2604 OID 21669)
-- Name: location id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.location ALTER COLUMN id SET DEFAULT nextval('public.location_id_seq'::regclass);


--
-- TOC entry 3904 (class 2604 OID 21670)
-- Name: mat_group id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.mat_group ALTER COLUMN id SET DEFAULT nextval('public.mat_group_id_seq'::regclass);


--
-- TOC entry 3908 (class 2604 OID 21671)
-- Name: mat_name id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.mat_name ALTER COLUMN id SET DEFAULT nextval('public.mat_name_id_seq'::regclass);


--
-- TOC entry 3912 (class 2604 OID 21672)
-- Name: material_code id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.material_code ALTER COLUMN id SET DEFAULT nextval('public.material_code_id_seq'::regclass);


--
-- TOC entry 3964 (class 2604 OID 21673)
-- Name: pr_attachment id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.pr_attachment ALTER COLUMN id SET DEFAULT nextval('public.pr_attachment_id_seq'::regclass);


--
-- TOC entry 3968 (class 2604 OID 21674)
-- Name: project id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.project ALTER COLUMN id SET DEFAULT nextval('public.project_id_seq'::regclass);


--
-- TOC entry 4018 (class 2604 OID 21675)
-- Name: requisition id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.requisition ALTER COLUMN id SET DEFAULT nextval('public.requisition_id_seq'::regclass);


--
-- TOC entry 4024 (class 2604 OID 21676)
-- Name: requisition_line id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.requisition_line ALTER COLUMN id SET DEFAULT nextval('public.requisition_line_id_seq'::regclass);


--
-- TOC entry 4026 (class 2604 OID 21677)
-- Name: requisition_status_log id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.requisition_status_log ALTER COLUMN id SET DEFAULT nextval('public.requisition_status_log_id_seq'::regclass);


--
-- TOC entry 4033 (class 2604 OID 21678)
-- Name: role_menu_permissions id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.role_menu_permissions ALTER COLUMN id SET DEFAULT nextval('public.role_menu_permissions_id_seq'::regclass);


--
-- TOC entry 4046 (class 2604 OID 21679)
-- Name: spec_size id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.spec_size ALTER COLUMN id SET DEFAULT nextval('public.spec_size_id_seq'::regclass);


--
-- TOC entry 4069 (class 2604 OID 21680)
-- Name: stock_item_image id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.stock_item_image ALTER COLUMN id SET DEFAULT nextval('public.stock_item_image_id_seq'::regclass);


--
-- TOC entry 4083 (class 2604 OID 21681)
-- Name: storage_zone id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.storage_zone ALTER COLUMN id SET DEFAULT nextval('public.storage_zone_id_seq'::regclass);


--
-- TOC entry 4086 (class 2604 OID 21682)
-- Name: subgroup id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.subgroup ALTER COLUMN id SET DEFAULT nextval('public.subgroup_id_seq'::regclass);


--
-- TOC entry 4090 (class 2604 OID 21683)
-- Name: supplier id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.supplier ALTER COLUMN id SET DEFAULT nextval('public.supplier_id_seq'::regclass);


--
-- TOC entry 4095 (class 2604 OID 21684)
-- Name: unit id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.unit ALTER COLUMN id SET DEFAULT nextval('public.unit_id_seq'::regclass);


--
-- TOC entry 4111 (class 2604 OID 21685)
-- Name: warehouse id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.warehouse ALTER COLUMN id SET DEFAULT nextval('public.warehouse_id_seq'::regclass);


--
-- TOC entry 4145 (class 2604 OID 21686)
-- Name: work_order_payment_installment id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.work_order_payment_installment ALTER COLUMN id SET DEFAULT nextval('public.work_order_payment_installment_id_seq'::regclass);


--
-- TOC entry 4150 (class 2604 OID 21687)
-- Name: work_order_penalty id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.work_order_penalty ALTER COLUMN id SET DEFAULT nextval('public.work_order_penalty_id_seq'::regclass);


--
-- TOC entry 4153 (class 2604 OID 21688)
-- Name: work_order_retention id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.work_order_retention ALTER COLUMN id SET DEFAULT nextval('public.work_order_retention_id_seq'::regclass);


--
-- TOC entry 4268 (class 2606 OID 21690)
-- Name: approval_config approval_config_doc_type_step_no_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.approval_config
    ADD CONSTRAINT approval_config_doc_type_step_no_key UNIQUE (doc_type, step_no);


--
-- TOC entry 4270 (class 2606 OID 21692)
-- Name: approval_config approval_config_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.approval_config
    ADD CONSTRAINT approval_config_pkey PRIMARY KEY (id);


--
-- TOC entry 4272 (class 2606 OID 21694)
-- Name: approval_delegation approval_delegation_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.approval_delegation
    ADD CONSTRAINT approval_delegation_pkey PRIMARY KEY (id);


--
-- TOC entry 4274 (class 2606 OID 21696)
-- Name: approval_doc_types approval_doc_types_doc_type_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.approval_doc_types
    ADD CONSTRAINT approval_doc_types_doc_type_key UNIQUE (doc_type);


--
-- TOC entry 4276 (class 2606 OID 21698)
-- Name: approval_doc_types approval_doc_types_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.approval_doc_types
    ADD CONSTRAINT approval_doc_types_pkey PRIMARY KEY (id);


--
-- TOC entry 4278 (class 2606 OID 21700)
-- Name: approval_log approval_log_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.approval_log
    ADD CONSTRAINT approval_log_pkey PRIMARY KEY (id);


--
-- TOC entry 4283 (class 2606 OID 21702)
-- Name: approval_request approval_request_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.approval_request
    ADD CONSTRAINT approval_request_pkey PRIMARY KEY (id);


--
-- TOC entry 4288 (class 2606 OID 21704)
-- Name: borrow borrow_borrow_no_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.borrow
    ADD CONSTRAINT borrow_borrow_no_key UNIQUE (borrow_no);


--
-- TOC entry 4293 (class 2606 OID 21706)
-- Name: borrow_line borrow_line_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.borrow_line
    ADD CONSTRAINT borrow_line_pkey PRIMARY KEY (id);


--
-- TOC entry 4290 (class 2606 OID 21708)
-- Name: borrow borrow_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.borrow
    ADD CONSTRAINT borrow_pkey PRIMARY KEY (id);


--
-- TOC entry 4295 (class 2606 OID 21710)
-- Name: borrow_status_log borrow_status_log_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.borrow_status_log
    ADD CONSTRAINT borrow_status_log_pkey PRIMARY KEY (id);


--
-- TOC entry 4298 (class 2606 OID 21712)
-- Name: brand brand_brand_code_spec_id_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.brand
    ADD CONSTRAINT brand_brand_code_spec_id_key UNIQUE (brand_code, spec_id);


--
-- TOC entry 4300 (class 2606 OID 21714)
-- Name: brand brand_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.brand
    ADD CONSTRAINT brand_pkey PRIMARY KEY (id);


--
-- TOC entry 4302 (class 2606 OID 21716)
-- Name: cost_group cost_group_job_id_group_code_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.cost_group
    ADD CONSTRAINT cost_group_job_id_group_code_key UNIQUE (job_id, group_code);


--
-- TOC entry 4304 (class 2606 OID 21718)
-- Name: cost_group cost_group_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.cost_group
    ADD CONSTRAINT cost_group_pkey PRIMARY KEY (id);


--
-- TOC entry 4307 (class 2606 OID 21720)
-- Name: cost_job cost_job_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.cost_job
    ADD CONSTRAINT cost_job_pkey PRIMARY KEY (id);


--
-- TOC entry 4309 (class 2606 OID 21722)
-- Name: cost_job cost_job_subject_id_job_code_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.cost_job
    ADD CONSTRAINT cost_job_subject_id_job_code_key UNIQUE (subject_id, job_code);


--
-- TOC entry 4312 (class 2606 OID 21724)
-- Name: cost_subgroup cost_subgroup_group_id_subgroup_code_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.cost_subgroup
    ADD CONSTRAINT cost_subgroup_group_id_subgroup_code_key UNIQUE (group_id, subgroup_code);


--
-- TOC entry 4314 (class 2606 OID 21726)
-- Name: cost_subgroup cost_subgroup_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.cost_subgroup
    ADD CONSTRAINT cost_subgroup_pkey PRIMARY KEY (id);


--
-- TOC entry 4317 (class 2606 OID 21728)
-- Name: cost_subject cost_subject_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.cost_subject
    ADD CONSTRAINT cost_subject_pkey PRIMARY KEY (id);


--
-- TOC entry 4319 (class 2606 OID 21730)
-- Name: cost_subject cost_subject_subject_code_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.cost_subject
    ADD CONSTRAINT cost_subject_subject_code_key UNIQUE (subject_code);


--
-- TOC entry 4321 (class 2606 OID 21732)
-- Name: customer customer_customer_code_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.customer
    ADD CONSTRAINT customer_customer_code_key UNIQUE (customer_code);


--
-- TOC entry 4323 (class 2606 OID 21734)
-- Name: customer customer_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.customer
    ADD CONSTRAINT customer_pkey PRIMARY KEY (cus_id);


--
-- TOC entry 4327 (class 2606 OID 21736)
-- Name: departments departments_dept_code_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.departments
    ADD CONSTRAINT departments_dept_code_key UNIQUE (dept_code);


--
-- TOC entry 4329 (class 2606 OID 21738)
-- Name: departments departments_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.departments
    ADD CONSTRAINT departments_pkey PRIMARY KEY (id);


--
-- TOC entry 4331 (class 2606 OID 21740)
-- Name: dept_menu_permissions dept_menu_permissions_dept_code_menu_id_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.dept_menu_permissions
    ADD CONSTRAINT dept_menu_permissions_dept_code_menu_id_key UNIQUE (dept_code, menu_id);


--
-- TOC entry 4333 (class 2606 OID 21742)
-- Name: dept_menu_permissions dept_menu_permissions_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.dept_menu_permissions
    ADD CONSTRAINT dept_menu_permissions_pkey PRIMARY KEY (id);


--
-- TOC entry 4696 (class 2606 OID 23742)
-- Name: doc_number_counter doc_number_counter_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.doc_number_counter
    ADD CONSTRAINT doc_number_counter_pkey PRIMARY KEY (prefix, year_month);


--
-- TOC entry 4335 (class 2606 OID 21744)
-- Name: erp_audit_log erp_audit_log_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.erp_audit_log
    ADD CONSTRAINT erp_audit_log_pkey PRIMARY KEY (id);


--
-- TOC entry 4339 (class 2606 OID 21746)
-- Name: grn grn_grn_no_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.grn
    ADD CONSTRAINT grn_grn_no_key UNIQUE (grn_no);


--
-- TOC entry 4345 (class 2606 OID 21748)
-- Name: grn_line grn_line_grn_id_line_no_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.grn_line
    ADD CONSTRAINT grn_line_grn_id_line_no_key UNIQUE (grn_id, line_no);


--
-- TOC entry 4347 (class 2606 OID 21750)
-- Name: grn_line grn_line_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.grn_line
    ADD CONSTRAINT grn_line_pkey PRIMARY KEY (id);


--
-- TOC entry 4341 (class 2606 OID 21752)
-- Name: grn grn_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.grn
    ADD CONSTRAINT grn_pkey PRIMARY KEY (id);


--
-- TOC entry 4349 (class 2606 OID 21754)
-- Name: ic_po_receive_document ic_po_receive_document_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.ic_po_receive_document
    ADD CONSTRAINT ic_po_receive_document_pkey PRIMARY KEY (id);


--
-- TOC entry 4679 (class 2606 OID 23413)
-- Name: ic_po_return_document ic_po_return_document_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.ic_po_return_document
    ADD CONSTRAINT ic_po_return_document_pkey PRIMARY KEY (id);


--
-- TOC entry 4681 (class 2606 OID 23415)
-- Name: ic_po_return_document ic_po_return_document_return_no_uq; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.ic_po_return_document
    ADD CONSTRAINT ic_po_return_document_return_no_uq UNIQUE (return_no);


--
-- TOC entry 4684 (class 2606 OID 23433)
-- Name: ic_po_return_line ic_po_return_line_no_uq; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.ic_po_return_line
    ADD CONSTRAINT ic_po_return_line_no_uq UNIQUE (return_id, line_no);


--
-- TOC entry 4686 (class 2606 OID 23431)
-- Name: ic_po_return_line ic_po_return_line_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.ic_po_return_line
    ADD CONSTRAINT ic_po_return_line_pkey PRIMARY KEY (id);


--
-- TOC entry 4353 (class 2606 OID 21756)
-- Name: ic_project_cost_item ic_project_cost_item_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.ic_project_cost_item
    ADD CONSTRAINT ic_project_cost_item_pkey PRIMARY KEY (id);


--
-- TOC entry 4355 (class 2606 OID 21758)
-- Name: ic_project_cost_item ic_project_cost_item_project_code_mat_code_cost_subgroup_id_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.ic_project_cost_item
    ADD CONSTRAINT ic_project_cost_item_project_code_mat_code_cost_subgroup_id_key UNIQUE (project_code, mat_code, cost_subgroup_id);


--
-- TOC entry 4358 (class 2606 OID 21760)
-- Name: ic_project_cost_item_transaction ic_project_cost_item_transaction_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.ic_project_cost_item_transaction
    ADD CONSTRAINT ic_project_cost_item_transaction_pkey PRIMARY KEY (id);


--
-- TOC entry 4364 (class 2606 OID 21762)
-- Name: ic_project_movement ic_project_movement_doc_no_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.ic_project_movement
    ADD CONSTRAINT ic_project_movement_doc_no_key UNIQUE (doc_no);


--
-- TOC entry 4369 (class 2606 OID 21764)
-- Name: ic_project_movement_line ic_project_movement_line_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.ic_project_movement_line
    ADD CONSTRAINT ic_project_movement_line_pkey PRIMARY KEY (id);


--
-- TOC entry 4366 (class 2606 OID 21766)
-- Name: ic_project_movement ic_project_movement_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.ic_project_movement
    ADD CONSTRAINT ic_project_movement_pkey PRIMARY KEY (id);


--
-- TOC entry 4372 (class 2606 OID 21768)
-- Name: ic_project_receipt_pool ic_project_receipt_pool_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.ic_project_receipt_pool
    ADD CONSTRAINT ic_project_receipt_pool_pkey PRIMARY KEY (id);


--
-- TOC entry 4377 (class 2606 OID 21770)
-- Name: ic_project_receipt_pool_transaction ic_project_receipt_pool_transaction_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.ic_project_receipt_pool_transaction
    ADD CONSTRAINT ic_project_receipt_pool_transaction_pkey PRIMARY KEY (id);


--
-- TOC entry 4382 (class 2606 OID 21772)
-- Name: inventory inventory_mat_code_warehouse_code_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.inventory
    ADD CONSTRAINT inventory_mat_code_warehouse_code_key UNIQUE (mat_code, warehouse_code);


--
-- TOC entry 4384 (class 2606 OID 21774)
-- Name: inventory inventory_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.inventory
    ADD CONSTRAINT inventory_pkey PRIMARY KEY (id);


--
-- TOC entry 4390 (class 2606 OID 21776)
-- Name: inventory_transaction inventory_transaction_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.inventory_transaction
    ADD CONSTRAINT inventory_transaction_pkey PRIMARY KEY (id);


--
-- TOC entry 4392 (class 2606 OID 21778)
-- Name: inventory_transaction inventory_transaction_txn_no_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.inventory_transaction
    ADD CONSTRAINT inventory_transaction_txn_no_key UNIQUE (txn_no);


--
-- TOC entry 4394 (class 2606 OID 21780)
-- Name: location location_location_code_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.location
    ADD CONSTRAINT location_location_code_key UNIQUE (location_code);


--
-- TOC entry 4396 (class 2606 OID 21782)
-- Name: location location_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.location
    ADD CONSTRAINT location_pkey PRIMARY KEY (id);


--
-- TOC entry 4398 (class 2606 OID 21784)
-- Name: mat_group mat_group_group_code_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.mat_group
    ADD CONSTRAINT mat_group_group_code_key UNIQUE (group_code);


--
-- TOC entry 4400 (class 2606 OID 21786)
-- Name: mat_group mat_group_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.mat_group
    ADD CONSTRAINT mat_group_pkey PRIMARY KEY (id);


--
-- TOC entry 4402 (class 2606 OID 21788)
-- Name: mat_name mat_name_mat_name_code_subgroup_id_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.mat_name
    ADD CONSTRAINT mat_name_mat_name_code_subgroup_id_key UNIQUE (mat_name_code, subgroup_id);


--
-- TOC entry 4404 (class 2606 OID 21790)
-- Name: mat_name mat_name_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.mat_name
    ADD CONSTRAINT mat_name_pkey PRIMARY KEY (id);


--
-- TOC entry 4414 (class 2606 OID 21792)
-- Name: material_code material_code_mat_code_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.material_code
    ADD CONSTRAINT material_code_mat_code_key UNIQUE (mat_code);


--
-- TOC entry 4416 (class 2606 OID 21794)
-- Name: material_code material_code_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.material_code
    ADD CONSTRAINT material_code_pkey PRIMARY KEY (id);


--
-- TOC entry 4425 (class 2606 OID 21796)
-- Name: memo_attachment memo_attachment_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.memo_attachment
    ADD CONSTRAINT memo_attachment_pkey PRIMARY KEY (id);


--
-- TOC entry 4428 (class 2606 OID 21798)
-- Name: memo_line memo_line_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.memo_line
    ADD CONSTRAINT memo_line_pkey PRIMARY KEY (id);


--
-- TOC entry 4430 (class 2606 OID 21800)
-- Name: memo_line memo_line_unique_line_no; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.memo_line
    ADD CONSTRAINT memo_line_unique_line_no UNIQUE (memo_id, line_no);


--
-- TOC entry 4420 (class 2606 OID 21802)
-- Name: memo memo_memo_no_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.memo
    ADD CONSTRAINT memo_memo_no_key UNIQUE (memo_no);


--
-- TOC entry 4432 (class 2606 OID 21804)
-- Name: memo_number_counter memo_number_counter_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.memo_number_counter
    ADD CONSTRAINT memo_number_counter_pkey PRIMARY KEY (year_month);


--
-- TOC entry 4422 (class 2606 OID 21806)
-- Name: memo memo_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.memo
    ADD CONSTRAINT memo_pkey PRIMARY KEY (id);


--
-- TOC entry 4435 (class 2606 OID 21808)
-- Name: memo_status_log memo_status_log_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.memo_status_log
    ADD CONSTRAINT memo_status_log_pkey PRIMARY KEY (id);


--
-- TOC entry 4437 (class 2606 OID 21810)
-- Name: menus menus_menu_code_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.menus
    ADD CONSTRAINT menus_menu_code_key UNIQUE (menu_code);


--
-- TOC entry 4439 (class 2606 OID 21812)
-- Name: menus menus_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.menus
    ADD CONSTRAINT menus_pkey PRIMARY KEY (id);


--
-- TOC entry 4441 (class 2606 OID 21814)
-- Name: modules modules_module_code_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.modules
    ADD CONSTRAINT modules_module_code_key UNIQUE (module_code);


--
-- TOC entry 4443 (class 2606 OID 21816)
-- Name: modules modules_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.modules
    ADD CONSTRAINT modules_pkey PRIMARY KEY (id);


--
-- TOC entry 4704 (class 2606 OID 25590)
-- Name: payment_installment payment_installment_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.payment_installment
    ADD CONSTRAINT payment_installment_pkey PRIMARY KEY (id);


--
-- TOC entry 4446 (class 2606 OID 21818)
-- Name: payment_log payment_log_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.payment_log
    ADD CONSTRAINT payment_log_pkey PRIMARY KEY (id);


--
-- TOC entry 4689 (class 2606 OID 23673)
-- Name: pc_number_counter pc_number_counter_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.pc_number_counter
    ADD CONSTRAINT pc_number_counter_pkey PRIMARY KEY (year_month);


--
-- TOC entry 4449 (class 2606 OID 21820)
-- Name: permission_audit_logs permission_audit_logs_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.permission_audit_logs
    ADD CONSTRAINT permission_audit_logs_pkey PRIMARY KEY (id);


--
-- TOC entry 4451 (class 2606 OID 21822)
-- Name: permissions permissions_permission_key_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.permissions
    ADD CONSTRAINT permissions_permission_key_key UNIQUE (permission_key);


--
-- TOC entry 4453 (class 2606 OID 21824)
-- Name: permissions permissions_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.permissions
    ADD CONSTRAINT permissions_pkey PRIMARY KEY (id);


--
-- TOC entry 4692 (class 2606 OID 23694)
-- Name: petty_cash_attachment petty_cash_attachment_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.petty_cash_attachment
    ADD CONSTRAINT petty_cash_attachment_pkey PRIMARY KEY (id);


--
-- TOC entry 4468 (class 2606 OID 21826)
-- Name: petty_cash_requisition_line petty_cash_requisition_line_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.petty_cash_requisition_line
    ADD CONSTRAINT petty_cash_requisition_line_pkey PRIMARY KEY (id);


--
-- TOC entry 4460 (class 2606 OID 21828)
-- Name: petty_cash_requisition petty_cash_requisition_pc_no_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.petty_cash_requisition
    ADD CONSTRAINT petty_cash_requisition_pc_no_key UNIQUE (pc_no);


--
-- TOC entry 4462 (class 2606 OID 21830)
-- Name: petty_cash_requisition petty_cash_requisition_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.petty_cash_requisition
    ADD CONSTRAINT petty_cash_requisition_pkey PRIMARY KEY (id);


--
-- TOC entry 4471 (class 2606 OID 21832)
-- Name: petty_cash_status_log petty_cash_status_log_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.petty_cash_status_log
    ADD CONSTRAINT petty_cash_status_log_pkey PRIMARY KEY (id);


--
-- TOC entry 4473 (class 2606 OID 21834)
-- Name: po_attachment po_attachment_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.po_attachment
    ADD CONSTRAINT po_attachment_pkey PRIMARY KEY (id);


--
-- TOC entry 4475 (class 2606 OID 21836)
-- Name: po_edit_log po_edit_log_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.po_edit_log
    ADD CONSTRAINT po_edit_log_pkey PRIMARY KEY (id);


--
-- TOC entry 4477 (class 2606 OID 21838)
-- Name: po_number_counter po_number_counter_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.po_number_counter
    ADD CONSTRAINT po_number_counter_pkey PRIMARY KEY (year_month);


--
-- TOC entry 4479 (class 2606 OID 21840)
-- Name: po_status_log po_status_log_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.po_status_log
    ADD CONSTRAINT po_status_log_pkey PRIMARY KEY (id);


--
-- TOC entry 4481 (class 2606 OID 21842)
-- Name: por_number_counter por_number_counter_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.por_number_counter
    ADD CONSTRAINT por_number_counter_pkey PRIMARY KEY (year_month);


--
-- TOC entry 4483 (class 2606 OID 21844)
-- Name: pr_attachment pr_attachment_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.pr_attachment
    ADD CONSTRAINT pr_attachment_pkey PRIMARY KEY (id);


--
-- TOC entry 4485 (class 2606 OID 21846)
-- Name: pr_number_counter pr_number_counter_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.pr_number_counter
    ADD CONSTRAINT pr_number_counter_pkey PRIMARY KEY (year_month);


--
-- TOC entry 4487 (class 2606 OID 21848)
-- Name: pr_status_log pr_status_log_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.pr_status_log
    ADD CONSTRAINT pr_status_log_pkey PRIMARY KEY (id);


--
-- TOC entry 4708 (class 2606 OID 25653)
-- Name: project_code_counter project_code_counter_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.project_code_counter
    ADD CONSTRAINT project_code_counter_pkey PRIMARY KEY (code_year, cus_id);


--
-- TOC entry 4713 (class 2606 OID 25802)
-- Name: project_code_year_counter project_code_year_counter_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.project_code_year_counter
    ADD CONSTRAINT project_code_year_counter_pkey PRIMARY KEY (code_year);


--
-- TOC entry 4718 (class 2606 OID 25858)
-- Name: project_cost_budget project_cost_budget_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.project_cost_budget
    ADD CONSTRAINT project_cost_budget_pkey PRIMARY KEY (id);


--
-- TOC entry 4715 (class 2606 OID 25815)
-- Name: project_job_share project_job_share_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.project_job_share
    ADD CONSTRAINT project_job_share_pkey PRIMARY KEY (project_id, job_code);


--
-- TOC entry 4491 (class 2606 OID 21850)
-- Name: project project_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.project
    ADD CONSTRAINT project_pkey PRIMARY KEY (id);


--
-- TOC entry 4493 (class 2606 OID 21852)
-- Name: project project_project_code_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.project
    ADD CONSTRAINT project_project_code_key UNIQUE (project_code);


--
-- TOC entry 4711 (class 2606 OID 25773)
-- Name: project_responsible_person project_responsible_person_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.project_responsible_person
    ADD CONSTRAINT project_responsible_person_pkey PRIMARY KEY (project_id, user_id);


--
-- TOC entry 4496 (class 2606 OID 21854)
-- Name: project_stock project_stock_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.project_stock
    ADD CONSTRAINT project_stock_pkey PRIMARY KEY (id);


--
-- TOC entry 4498 (class 2606 OID 21856)
-- Name: project_stock project_stock_project_code_mat_code_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.project_stock
    ADD CONSTRAINT project_stock_project_code_mat_code_key UNIQUE (project_code, mat_code);


--
-- TOC entry 4512 (class 2606 OID 21858)
-- Name: purchase_order_line purchase_order_line_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.purchase_order_line
    ADD CONSTRAINT purchase_order_line_pkey PRIMARY KEY (id);


--
-- TOC entry 4514 (class 2606 OID 21860)
-- Name: purchase_order_line purchase_order_line_po_id_line_no_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.purchase_order_line
    ADD CONSTRAINT purchase_order_line_po_id_line_no_key UNIQUE (po_id, line_no);


--
-- TOC entry 4507 (class 2606 OID 21862)
-- Name: purchase_order purchase_order_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.purchase_order
    ADD CONSTRAINT purchase_order_pkey PRIMARY KEY (id);


--
-- TOC entry 4509 (class 2606 OID 21864)
-- Name: purchase_order purchase_order_po_no_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.purchase_order
    ADD CONSTRAINT purchase_order_po_no_key UNIQUE (po_no);


--
-- TOC entry 4525 (class 2606 OID 21866)
-- Name: purchase_request_line purchase_request_line_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.purchase_request_line
    ADD CONSTRAINT purchase_request_line_pkey PRIMARY KEY (id);


--
-- TOC entry 4527 (class 2606 OID 21868)
-- Name: purchase_request_line purchase_request_line_pr_id_line_no_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.purchase_request_line
    ADD CONSTRAINT purchase_request_line_pr_id_line_no_key UNIQUE (pr_id, line_no);


--
-- TOC entry 4520 (class 2606 OID 21870)
-- Name: purchase_request purchase_request_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.purchase_request
    ADD CONSTRAINT purchase_request_pkey PRIMARY KEY (id);


--
-- TOC entry 4522 (class 2606 OID 21872)
-- Name: purchase_request purchase_request_pr_no_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.purchase_request
    ADD CONSTRAINT purchase_request_pr_no_key UNIQUE (pr_no);


--
-- TOC entry 4538 (class 2606 OID 21874)
-- Name: requisition_line requisition_line_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.requisition_line
    ADD CONSTRAINT requisition_line_pkey PRIMARY KEY (id);


--
-- TOC entry 4532 (class 2606 OID 21876)
-- Name: requisition requisition_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.requisition
    ADD CONSTRAINT requisition_pkey PRIMARY KEY (id);


--
-- TOC entry 4534 (class 2606 OID 21878)
-- Name: requisition requisition_req_no_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.requisition
    ADD CONSTRAINT requisition_req_no_key UNIQUE (req_no);


--
-- TOC entry 4540 (class 2606 OID 21880)
-- Name: requisition_status_log requisition_status_log_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.requisition_status_log
    ADD CONSTRAINT requisition_status_log_pkey PRIMARY KEY (id);


--
-- TOC entry 4694 (class 2606 OID 23734)
-- Name: ret_number_counter ret_number_counter_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.ret_number_counter
    ADD CONSTRAINT ret_number_counter_pkey PRIMARY KEY (year_month);


--
-- TOC entry 4546 (class 2606 OID 21882)
-- Name: rfq_line rfq_line_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.rfq_line
    ADD CONSTRAINT rfq_line_pkey PRIMARY KEY (id);


--
-- TOC entry 4548 (class 2606 OID 21884)
-- Name: rfq_line rfq_line_rfq_id_line_no_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.rfq_line
    ADD CONSTRAINT rfq_line_rfq_id_line_no_key UNIQUE (rfq_id, line_no);


--
-- TOC entry 4542 (class 2606 OID 21886)
-- Name: rfq rfq_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.rfq
    ADD CONSTRAINT rfq_pkey PRIMARY KEY (id);


--
-- TOC entry 4544 (class 2606 OID 21888)
-- Name: rfq rfq_rfq_no_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.rfq
    ADD CONSTRAINT rfq_rfq_no_key UNIQUE (rfq_no);


--
-- TOC entry 4550 (class 2606 OID 21890)
-- Name: role_menu_permissions role_menu_permissions_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.role_menu_permissions
    ADD CONSTRAINT role_menu_permissions_pkey PRIMARY KEY (id);


--
-- TOC entry 4552 (class 2606 OID 21892)
-- Name: role_menu_permissions role_menu_permissions_role_id_menu_id_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.role_menu_permissions
    ADD CONSTRAINT role_menu_permissions_role_id_menu_id_key UNIQUE (role_id, menu_id);


--
-- TOC entry 4554 (class 2606 OID 21894)
-- Name: role_menus role_menus_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.role_menus
    ADD CONSTRAINT role_menus_pkey PRIMARY KEY (role_id, menu_id);


--
-- TOC entry 4556 (class 2606 OID 21896)
-- Name: role_permissions role_permissions_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.role_permissions
    ADD CONSTRAINT role_permissions_pkey PRIMARY KEY (role_id, permission_id);


--
-- TOC entry 4559 (class 2606 OID 21898)
-- Name: roles roles_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.roles
    ADD CONSTRAINT roles_pkey PRIMARY KEY (id);


--
-- TOC entry 4561 (class 2606 OID 21900)
-- Name: roles roles_role_code_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.roles
    ADD CONSTRAINT roles_role_code_key UNIQUE (role_code);


--
-- TOC entry 4563 (class 2606 OID 21902)
-- Name: spec_size spec_size_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.spec_size
    ADD CONSTRAINT spec_size_pkey PRIMARY KEY (id);


--
-- TOC entry 4565 (class 2606 OID 21904)
-- Name: spec_size spec_size_spec_code_mat_name_id_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.spec_size
    ADD CONSTRAINT spec_size_spec_code_mat_name_id_key UNIQUE (spec_code, mat_name_id);


--
-- TOC entry 4567 (class 2606 OID 21906)
-- Name: stock_category stock_category_code_uq; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.stock_category
    ADD CONSTRAINT stock_category_code_uq UNIQUE (code);


--
-- TOC entry 4569 (class 2606 OID 21908)
-- Name: stock_category stock_category_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.stock_category
    ADD CONSTRAINT stock_category_pkey PRIMARY KEY (id);


--
-- TOC entry 4571 (class 2606 OID 21910)
-- Name: stock_count stock_count_count_no_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.stock_count
    ADD CONSTRAINT stock_count_count_no_key UNIQUE (count_no);


--
-- TOC entry 4575 (class 2606 OID 21912)
-- Name: stock_count_line stock_count_line_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.stock_count_line
    ADD CONSTRAINT stock_count_line_pkey PRIMARY KEY (id);


--
-- TOC entry 4573 (class 2606 OID 21914)
-- Name: stock_count stock_count_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.stock_count
    ADD CONSTRAINT stock_count_pkey PRIMARY KEY (id);


--
-- TOC entry 4580 (class 2606 OID 21916)
-- Name: stock_inventory stock_inventory_item_loc_uq; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.stock_inventory
    ADD CONSTRAINT stock_inventory_item_loc_uq UNIQUE (item_id, location_code);


--
-- TOC entry 4582 (class 2606 OID 21918)
-- Name: stock_inventory stock_inventory_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.stock_inventory
    ADD CONSTRAINT stock_inventory_pkey PRIMARY KEY (id);


--
-- TOC entry 4587 (class 2606 OID 21920)
-- Name: stock_item stock_item_code_uq; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.stock_item
    ADD CONSTRAINT stock_item_code_uq UNIQUE (mat_code);


--
-- TOC entry 4592 (class 2606 OID 21922)
-- Name: stock_item_image stock_item_image_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.stock_item_image
    ADD CONSTRAINT stock_item_image_pkey PRIMARY KEY (id);


--
-- TOC entry 4589 (class 2606 OID 21924)
-- Name: stock_item stock_item_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.stock_item
    ADD CONSTRAINT stock_item_pkey PRIMARY KEY (id);


--
-- TOC entry 4598 (class 2606 OID 21926)
-- Name: stock_reservation stock_res_no_uq; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.stock_reservation
    ADD CONSTRAINT stock_res_no_uq UNIQUE (reservation_no);


--
-- TOC entry 4600 (class 2606 OID 21928)
-- Name: stock_reservation stock_res_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.stock_reservation
    ADD CONSTRAINT stock_res_pkey PRIMARY KEY (id);


--
-- TOC entry 4617 (class 2606 OID 21930)
-- Name: stock_transfer_line stock_transfer_line_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.stock_transfer_line
    ADD CONSTRAINT stock_transfer_line_pkey PRIMARY KEY (id);


--
-- TOC entry 4612 (class 2606 OID 21932)
-- Name: stock_transfer stock_transfer_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.stock_transfer
    ADD CONSTRAINT stock_transfer_pkey PRIMARY KEY (id);


--
-- TOC entry 4619 (class 2606 OID 21934)
-- Name: stock_transfer_status_log stock_transfer_status_log_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.stock_transfer_status_log
    ADD CONSTRAINT stock_transfer_status_log_pkey PRIMARY KEY (id);


--
-- TOC entry 4614 (class 2606 OID 23829)
-- Name: stock_transfer stock_transfer_transfer_no_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.stock_transfer
    ADD CONSTRAINT stock_transfer_transfer_no_key UNIQUE (transfer_no);


--
-- TOC entry 4608 (class 2606 OID 21936)
-- Name: stock_transaction stock_txn_no_uq; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.stock_transaction
    ADD CONSTRAINT stock_txn_no_uq UNIQUE (txn_no);


--
-- TOC entry 4610 (class 2606 OID 21938)
-- Name: stock_transaction stock_txn_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.stock_transaction
    ADD CONSTRAINT stock_txn_pkey PRIMARY KEY (id);


--
-- TOC entry 4621 (class 2606 OID 21940)
-- Name: storage_zone storage_zone_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.storage_zone
    ADD CONSTRAINT storage_zone_pkey PRIMARY KEY (id);


--
-- TOC entry 4623 (class 2606 OID 21942)
-- Name: storage_zone storage_zone_warehouse_id_zone_code_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.storage_zone
    ADD CONSTRAINT storage_zone_warehouse_id_zone_code_key UNIQUE (warehouse_id, zone_code);


--
-- TOC entry 4625 (class 2606 OID 21944)
-- Name: subgroup subgroup_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.subgroup
    ADD CONSTRAINT subgroup_pkey PRIMARY KEY (id);


--
-- TOC entry 4627 (class 2606 OID 21946)
-- Name: subgroup subgroup_subgroup_code_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.subgroup
    ADD CONSTRAINT subgroup_subgroup_code_key UNIQUE (subgroup_code);


--
-- TOC entry 4629 (class 2606 OID 21948)
-- Name: supplier supplier_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.supplier
    ADD CONSTRAINT supplier_pkey PRIMARY KEY (id);


--
-- TOC entry 4631 (class 2606 OID 21950)
-- Name: unit unit_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.unit
    ADD CONSTRAINT unit_pkey PRIMARY KEY (id);


--
-- TOC entry 4633 (class 2606 OID 21952)
-- Name: unit unit_unit_code_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.unit
    ADD CONSTRAINT unit_unit_code_key UNIQUE (unit_code);


--
-- TOC entry 4375 (class 2606 OID 21954)
-- Name: ic_project_receipt_pool uq_ic_project_receipt_pool; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.ic_project_receipt_pool
    ADD CONSTRAINT uq_ic_project_receipt_pool UNIQUE (project_code, mat_code, cost_subgroup_id);


--
-- TOC entry 4706 (class 2606 OID 25592)
-- Name: payment_installment uq_payment_installment_doc_no; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.payment_installment
    ADD CONSTRAINT uq_payment_installment_doc_no UNIQUE (doc_type, doc_id, installment_no);


--
-- TOC entry 4720 (class 2606 OID 25860)
-- Name: project_cost_budget uq_project_cost_budget_project_subgroup; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.project_cost_budget
    ADD CONSTRAINT uq_project_cost_budget_project_subgroup UNIQUE (project_id, cost_subgroup_id);


--
-- TOC entry 4661 (class 2606 OID 21956)
-- Name: work_order_cost_code uq_wo_cost_code_wo_id_code; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.work_order_cost_code
    ADD CONSTRAINT uq_wo_cost_code_wo_id_code UNIQUE (wo_id, cost_code);


--
-- TOC entry 4635 (class 2606 OID 21958)
-- Name: user_menu_permissions user_menu_permissions_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.user_menu_permissions
    ADD CONSTRAINT user_menu_permissions_pkey PRIMARY KEY (id);


--
-- TOC entry 4637 (class 2606 OID 21960)
-- Name: user_menu_permissions user_menu_permissions_user_id_menu_id_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.user_menu_permissions
    ADD CONSTRAINT user_menu_permissions_user_id_menu_id_key UNIQUE (user_id, menu_id);


--
-- TOC entry 4639 (class 2606 OID 21962)
-- Name: user_permissions user_permissions_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.user_permissions
    ADD CONSTRAINT user_permissions_pkey PRIMARY KEY (user_id, permission_id);


--
-- TOC entry 4641 (class 2606 OID 21964)
-- Name: user_roles user_roles_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.user_roles
    ADD CONSTRAINT user_roles_pkey PRIMARY KEY (user_id, role_id);


--
-- TOC entry 4644 (class 2606 OID 21966)
-- Name: users users_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_pkey PRIMARY KEY (id);


--
-- TOC entry 4646 (class 2606 OID 21968)
-- Name: users users_username_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_username_key UNIQUE (username);


--
-- TOC entry 4648 (class 2606 OID 21970)
-- Name: warehouse warehouse_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.warehouse
    ADD CONSTRAINT warehouse_pkey PRIMARY KEY (id);


--
-- TOC entry 4650 (class 2606 OID 21972)
-- Name: warehouse warehouse_warehouse_code_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.warehouse
    ADD CONSTRAINT warehouse_warehouse_code_key UNIQUE (warehouse_code);


--
-- TOC entry 4698 (class 2606 OID 23753)
-- Name: wht_income_type wht_income_type_code_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.wht_income_type
    ADD CONSTRAINT wht_income_type_code_key UNIQUE (code);


--
-- TOC entry 4700 (class 2606 OID 23751)
-- Name: wht_income_type wht_income_type_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.wht_income_type
    ADD CONSTRAINT wht_income_type_pkey PRIMARY KEY (id);


--
-- TOC entry 4658 (class 2606 OID 21974)
-- Name: work_order_attachment work_order_attachment_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.work_order_attachment
    ADD CONSTRAINT work_order_attachment_pkey PRIMARY KEY (id);


--
-- TOC entry 4663 (class 2606 OID 21976)
-- Name: work_order_cost_code work_order_cost_code_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.work_order_cost_code
    ADD CONSTRAINT work_order_cost_code_pkey PRIMARY KEY (id);


--
-- TOC entry 4723 (class 2606 OID 26461)
-- Name: work_order_edit_log work_order_edit_log_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.work_order_edit_log
    ADD CONSTRAINT work_order_edit_log_pkey PRIMARY KEY (id);


--
-- TOC entry 4666 (class 2606 OID 21978)
-- Name: work_order_line work_order_line_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.work_order_line
    ADD CONSTRAINT work_order_line_pkey PRIMARY KEY (id);


--
-- TOC entry 4669 (class 2606 OID 21980)
-- Name: work_order_payment_installment work_order_payment_installment_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.work_order_payment_installment
    ADD CONSTRAINT work_order_payment_installment_pkey PRIMARY KEY (id);


--
-- TOC entry 4672 (class 2606 OID 21982)
-- Name: work_order_penalty work_order_penalty_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.work_order_penalty
    ADD CONSTRAINT work_order_penalty_pkey PRIMARY KEY (id);


--
-- TOC entry 4654 (class 2606 OID 21984)
-- Name: work_order work_order_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.work_order
    ADD CONSTRAINT work_order_pkey PRIMARY KEY (id);


--
-- TOC entry 4675 (class 2606 OID 21986)
-- Name: work_order_retention work_order_retention_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.work_order_retention
    ADD CONSTRAINT work_order_retention_pkey PRIMARY KEY (id);


--
-- TOC entry 4677 (class 2606 OID 21988)
-- Name: work_order_status_log work_order_status_log_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.work_order_status_log
    ADD CONSTRAINT work_order_status_log_pkey PRIMARY KEY (id);


--
-- TOC entry 4656 (class 2606 OID 21990)
-- Name: work_order work_order_wo_no_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.work_order
    ADD CONSTRAINT work_order_wo_no_key UNIQUE (wo_no);


--
-- TOC entry 4279 (class 1259 OID 21991)
-- Name: idx_approval_log_at; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_approval_log_at ON public.approval_log USING btree (action_at);


--
-- TOC entry 4280 (class 1259 OID 21992)
-- Name: idx_approval_log_by; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_approval_log_by ON public.approval_log USING btree (action_by);


--
-- TOC entry 4281 (class 1259 OID 21993)
-- Name: idx_approval_log_doc; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_approval_log_doc ON public.approval_log USING btree (doc_type, doc_id);


--
-- TOC entry 4284 (class 1259 OID 21994)
-- Name: idx_approval_req_assignee; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_approval_req_assignee ON public.approval_request USING btree (assigned_to);


--
-- TOC entry 4285 (class 1259 OID 21995)
-- Name: idx_approval_req_doc; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_approval_req_doc ON public.approval_request USING btree (doc_type, doc_id);


--
-- TOC entry 4286 (class 1259 OID 21996)
-- Name: idx_approval_req_status; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_approval_req_status ON public.approval_request USING btree (status);


--
-- TOC entry 4291 (class 1259 OID 21997)
-- Name: idx_borrow_status; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_borrow_status ON public.borrow USING btree (status);


--
-- TOC entry 4296 (class 1259 OID 21998)
-- Name: idx_borrow_status_log_borrow; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_borrow_status_log_borrow ON public.borrow_status_log USING btree (borrow_id);


--
-- TOC entry 4305 (class 1259 OID 21999)
-- Name: idx_cost_group_job_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_cost_group_job_id ON public.cost_group USING btree (job_id);


--
-- TOC entry 4359 (class 1259 OID 23461)
-- Name: idx_cost_item_txn_return_document_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_cost_item_txn_return_document_id ON public.ic_project_cost_item_transaction USING btree (return_document_id);


--
-- TOC entry 4310 (class 1259 OID 22000)
-- Name: idx_cost_job_subject_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_cost_job_subject_id ON public.cost_job USING btree (subject_id);


--
-- TOC entry 4315 (class 1259 OID 22001)
-- Name: idx_cost_subgroup_group_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_cost_subgroup_group_id ON public.cost_subgroup USING btree (group_id);


--
-- TOC entry 4324 (class 1259 OID 22002)
-- Name: idx_customer_is_active; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_customer_is_active ON public.customer USING btree (is_active);


--
-- TOC entry 4325 (class 1259 OID 22003)
-- Name: idx_customer_name; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_customer_name ON public.customer USING btree (customer_name);


--
-- TOC entry 4336 (class 1259 OID 22004)
-- Name: idx_erp_audit_at; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_erp_audit_at ON public.erp_audit_log USING btree (changed_at);


--
-- TOC entry 4337 (class 1259 OID 22005)
-- Name: idx_erp_audit_table; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_erp_audit_table ON public.erp_audit_log USING btree (table_name, record_id);


--
-- TOC entry 4342 (class 1259 OID 22006)
-- Name: idx_grn_date; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_grn_date ON public.grn USING btree (grn_date);


--
-- TOC entry 4343 (class 1259 OID 22007)
-- Name: idx_grn_po; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_grn_po ON public.grn USING btree (po_id);


--
-- TOC entry 4360 (class 1259 OID 22008)
-- Name: idx_ic_pcit_receive_document_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_ic_pcit_receive_document_id ON public.ic_project_cost_item_transaction USING btree (receive_document_id);


--
-- TOC entry 4350 (class 1259 OID 22009)
-- Name: idx_ic_po_receive_document_po_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_ic_po_receive_document_po_id ON public.ic_po_receive_document USING btree (po_id);


--
-- TOC entry 4682 (class 1259 OID 23421)
-- Name: idx_ic_po_return_document_po_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_ic_po_return_document_po_id ON public.ic_po_return_document USING btree (po_id);


--
-- TOC entry 4687 (class 1259 OID 23449)
-- Name: idx_ic_po_return_line_return_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_ic_po_return_line_return_id ON public.ic_po_return_line USING btree (return_id);


--
-- TOC entry 4356 (class 1259 OID 22010)
-- Name: idx_ic_project_cost_item_project_code; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_ic_project_cost_item_project_code ON public.ic_project_cost_item USING btree (project_code);


--
-- TOC entry 4361 (class 1259 OID 22011)
-- Name: idx_ic_project_cost_item_txn_movement; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_ic_project_cost_item_txn_movement ON public.ic_project_cost_item_transaction USING btree (movement_id);


--
-- TOC entry 4362 (class 1259 OID 22012)
-- Name: idx_ic_project_cost_item_txn_po_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_ic_project_cost_item_txn_po_id ON public.ic_project_cost_item_transaction USING btree (po_id);


--
-- TOC entry 4370 (class 1259 OID 22013)
-- Name: idx_ic_project_movement_line_movement; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_ic_project_movement_line_movement ON public.ic_project_movement_line USING btree (movement_id);


--
-- TOC entry 4367 (class 1259 OID 22014)
-- Name: idx_ic_project_movement_project; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_ic_project_movement_project ON public.ic_project_movement USING btree (project_code);


--
-- TOC entry 4373 (class 1259 OID 22015)
-- Name: idx_ic_project_receipt_pool_project; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_ic_project_receipt_pool_project ON public.ic_project_receipt_pool USING btree (project_code);


--
-- TOC entry 4378 (class 1259 OID 22016)
-- Name: idx_ic_project_receipt_pool_txn_pool; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_ic_project_receipt_pool_txn_pool ON public.ic_project_receipt_pool_transaction USING btree (receipt_pool_id);


--
-- TOC entry 4385 (class 1259 OID 22017)
-- Name: idx_inv_txn_date; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_inv_txn_date ON public.inventory_transaction USING btree (txn_date);


--
-- TOC entry 4386 (class 1259 OID 22018)
-- Name: idx_inv_txn_mat; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_inv_txn_mat ON public.inventory_transaction USING btree (mat_code);


--
-- TOC entry 4387 (class 1259 OID 22019)
-- Name: idx_inv_txn_ref; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_inv_txn_ref ON public.inventory_transaction USING btree (ref_doc_type, ref_doc_no);


--
-- TOC entry 4388 (class 1259 OID 22020)
-- Name: idx_inv_txn_type; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_inv_txn_type ON public.inventory_transaction USING btree (txn_type);


--
-- TOC entry 4379 (class 1259 OID 22021)
-- Name: idx_inventory_mat; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_inventory_mat ON public.inventory USING btree (mat_code);


--
-- TOC entry 4380 (class 1259 OID 22022)
-- Name: idx_inventory_wh; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_inventory_wh ON public.inventory USING btree (warehouse_code);


--
-- TOC entry 4405 (class 1259 OID 22023)
-- Name: idx_material_code_brand_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_material_code_brand_id ON public.material_code USING btree (brand_id);


--
-- TOC entry 4406 (class 1259 OID 22024)
-- Name: idx_material_code_group_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_material_code_group_id ON public.material_code USING btree (group_id);


--
-- TOC entry 4407 (class 1259 OID 22025)
-- Name: idx_material_code_mat_code; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_material_code_mat_code ON public.material_code USING btree (mat_code);


--
-- TOC entry 4408 (class 1259 OID 22026)
-- Name: idx_material_code_mat_name_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_material_code_mat_name_id ON public.material_code USING btree (mat_name_id);


--
-- TOC entry 4409 (class 1259 OID 22027)
-- Name: idx_material_code_spec_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_material_code_spec_id ON public.material_code USING btree (spec_id);


--
-- TOC entry 4410 (class 1259 OID 22028)
-- Name: idx_material_code_subgroup_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_material_code_subgroup_id ON public.material_code USING btree (subgroup_id);


--
-- TOC entry 4411 (class 1259 OID 22029)
-- Name: idx_material_code_unit_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_material_code_unit_id ON public.material_code USING btree (unit_id);


--
-- TOC entry 4412 (class 1259 OID 22030)
-- Name: idx_material_cost_subgroup; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_material_cost_subgroup ON public.material_code USING btree (cost_subgroup_id);


--
-- TOC entry 4417 (class 1259 OID 22031)
-- Name: idx_memo_approver_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_memo_approver_id ON public.memo USING btree (approver_id);


--
-- TOC entry 4423 (class 1259 OID 22032)
-- Name: idx_memo_attachment_memo_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_memo_attachment_memo_id ON public.memo_attachment USING btree (memo_id);


--
-- TOC entry 4426 (class 1259 OID 22033)
-- Name: idx_memo_line_memo_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_memo_line_memo_id ON public.memo_line USING btree (memo_id);


--
-- TOC entry 4418 (class 1259 OID 22034)
-- Name: idx_memo_status; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_memo_status ON public.memo USING btree (status);


--
-- TOC entry 4433 (class 1259 OID 22035)
-- Name: idx_memo_status_log_memo_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_memo_status_log_memo_id ON public.memo_status_log USING btree (memo_id);


--
-- TOC entry 4701 (class 1259 OID 25598)
-- Name: idx_payment_installment_doc; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_payment_installment_doc ON public.payment_installment USING btree (doc_type, doc_id);


--
-- TOC entry 4702 (class 1259 OID 25633)
-- Name: idx_payment_installment_payment_log; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_payment_installment_payment_log ON public.payment_installment USING btree (payment_log_id);


--
-- TOC entry 4444 (class 1259 OID 22036)
-- Name: idx_payment_log_doc; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_payment_log_doc ON public.payment_log USING btree (doc_type, doc_id);


--
-- TOC entry 4454 (class 1259 OID 22037)
-- Name: idx_pc_requested_by; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_pc_requested_by ON public.petty_cash_requisition USING btree (requested_by);


--
-- TOC entry 4455 (class 1259 OID 22038)
-- Name: idx_pc_status; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_pc_status ON public.petty_cash_requisition USING btree (status);


--
-- TOC entry 4469 (class 1259 OID 22039)
-- Name: idx_pc_status_log_pc_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_pc_status_log_pc_id ON public.petty_cash_status_log USING btree (pc_id);


--
-- TOC entry 4463 (class 1259 OID 22040)
-- Name: idx_pcl_mat_code; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_pcl_mat_code ON public.petty_cash_requisition_line USING btree (mat_code);


--
-- TOC entry 4464 (class 1259 OID 22041)
-- Name: idx_pcl_pc_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_pcl_pc_id ON public.petty_cash_requisition_line USING btree (pc_id);


--
-- TOC entry 4465 (class 1259 OID 22042)
-- Name: idx_pcl_project_code; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_pcl_project_code ON public.petty_cash_requisition_line USING btree (project_code);


--
-- TOC entry 4466 (class 1259 OID 23783)
-- Name: idx_pcl_supplier_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_pcl_supplier_id ON public.petty_cash_requisition_line USING btree (supplier_id);


--
-- TOC entry 4456 (class 1259 OID 23683)
-- Name: idx_pcr_pc_date; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_pcr_pc_date ON public.petty_cash_requisition USING btree (pc_date);


--
-- TOC entry 4457 (class 1259 OID 23681)
-- Name: idx_pcr_project_code; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_pcr_project_code ON public.petty_cash_requisition USING btree (project_code);


--
-- TOC entry 4458 (class 1259 OID 23682)
-- Name: idx_pcr_status; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_pcr_status ON public.petty_cash_requisition USING btree (status);


--
-- TOC entry 4690 (class 1259 OID 23705)
-- Name: idx_petty_cash_attachment_pc_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_petty_cash_attachment_pc_id ON public.petty_cash_attachment USING btree (pc_id);


--
-- TOC entry 4499 (class 1259 OID 22043)
-- Name: idx_po_date; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_po_date ON public.purchase_order USING btree (po_date);


--
-- TOC entry 4510 (class 1259 OID 22044)
-- Name: idx_po_line_pr_line_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_po_line_pr_line_id ON public.purchase_order_line USING btree (pr_line_id);


--
-- TOC entry 4500 (class 1259 OID 22045)
-- Name: idx_po_pr_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_po_pr_id ON public.purchase_order USING btree (pr_id);


--
-- TOC entry 4501 (class 1259 OID 23784)
-- Name: idx_po_project_code; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_po_project_code ON public.purchase_order USING btree (project_code) WHERE (deleted_at IS NULL);


--
-- TOC entry 4502 (class 1259 OID 22046)
-- Name: idx_po_status; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_po_status ON public.purchase_order USING btree (status);


--
-- TOC entry 4503 (class 1259 OID 22047)
-- Name: idx_po_use_vat; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_po_use_vat ON public.purchase_order USING btree (use_vat);


--
-- TOC entry 4504 (class 1259 OID 22048)
-- Name: idx_po_use_wht; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_po_use_wht ON public.purchase_order USING btree (use_wht);


--
-- TOC entry 4505 (class 1259 OID 22049)
-- Name: idx_po_warehouse_code; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_po_warehouse_code ON public.purchase_order USING btree (warehouse_code);


--
-- TOC entry 4515 (class 1259 OID 22050)
-- Name: idx_pr_date; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_pr_date ON public.purchase_request USING btree (pr_date);


--
-- TOC entry 4516 (class 1259 OID 22051)
-- Name: idx_pr_memo_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_pr_memo_id ON public.purchase_request USING btree (memo_id);


--
-- TOC entry 4517 (class 1259 OID 22052)
-- Name: idx_pr_requested_by; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_pr_requested_by ON public.purchase_request USING btree (requested_by);


--
-- TOC entry 4518 (class 1259 OID 22053)
-- Name: idx_pr_status; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_pr_status ON public.purchase_request USING btree (status);


--
-- TOC entry 4523 (class 1259 OID 23785)
-- Name: idx_prl_memo_line_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_prl_memo_line_id ON public.purchase_request_line USING btree (memo_line_id);


--
-- TOC entry 4716 (class 1259 OID 25871)
-- Name: idx_project_cost_budget_project_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_project_cost_budget_project_id ON public.project_cost_budget USING btree (project_id);


--
-- TOC entry 4488 (class 1259 OID 22054)
-- Name: idx_project_owner_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_project_owner_id ON public.project USING btree (owner_id);


--
-- TOC entry 4494 (class 1259 OID 22055)
-- Name: idx_project_stock_lookup; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_project_stock_lookup ON public.project_stock USING btree (project_code, mat_code);


--
-- TOC entry 4709 (class 1259 OID 25784)
-- Name: idx_prp_user_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_prp_user_id ON public.project_responsible_person USING btree (user_id);


--
-- TOC entry 4535 (class 1259 OID 22056)
-- Name: idx_requisition_line_mat; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_requisition_line_mat ON public.requisition_line USING btree (mat_code);


--
-- TOC entry 4536 (class 1259 OID 22057)
-- Name: idx_requisition_line_req; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_requisition_line_req ON public.requisition_line USING btree (req_id);


--
-- TOC entry 4528 (class 1259 OID 22058)
-- Name: idx_requisition_project; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_requisition_project ON public.requisition USING btree (project_code);


--
-- TOC entry 4529 (class 1259 OID 22059)
-- Name: idx_requisition_requester; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_requisition_requester ON public.requisition USING btree (requester_id);


--
-- TOC entry 4530 (class 1259 OID 22060)
-- Name: idx_requisition_status; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_requisition_status ON public.requisition USING btree (status);


--
-- TOC entry 4557 (class 1259 OID 22061)
-- Name: idx_roles_dept_code; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_roles_dept_code ON public.roles USING btree (dept_code);


--
-- TOC entry 4576 (class 1259 OID 22062)
-- Name: idx_stock_inv_item; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_stock_inv_item ON public.stock_inventory USING btree (item_id);


--
-- TOC entry 4577 (class 1259 OID 22063)
-- Name: idx_stock_inv_location; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_stock_inv_location ON public.stock_inventory USING btree (location_code);


--
-- TOC entry 4578 (class 1259 OID 22064)
-- Name: idx_stock_inv_warehouse; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_stock_inv_warehouse ON public.stock_inventory USING btree (warehouse_code);


--
-- TOC entry 4583 (class 1259 OID 22065)
-- Name: idx_stock_item_category; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_stock_item_category ON public.stock_item USING btree (category_id);


--
-- TOC entry 4590 (class 1259 OID 22066)
-- Name: idx_stock_item_image_item_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_stock_item_image_item_id ON public.stock_item_image USING btree (item_id);


--
-- TOC entry 4584 (class 1259 OID 22067)
-- Name: idx_stock_item_is_active; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_stock_item_is_active ON public.stock_item USING btree (is_active);


--
-- TOC entry 4585 (class 1259 OID 22068)
-- Name: idx_stock_item_item_type; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_stock_item_item_type ON public.stock_item USING btree (item_type);


--
-- TOC entry 4593 (class 1259 OID 22069)
-- Name: idx_stock_res_item; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_stock_res_item ON public.stock_reservation USING btree (item_id);


--
-- TOC entry 4594 (class 1259 OID 22070)
-- Name: idx_stock_res_needed; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_stock_res_needed ON public.stock_reservation USING btree (needed_by);


--
-- TOC entry 4595 (class 1259 OID 22071)
-- Name: idx_stock_res_status; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_stock_res_status ON public.stock_reservation USING btree (status);


--
-- TOC entry 4596 (class 1259 OID 22072)
-- Name: idx_stock_res_user; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_stock_res_user ON public.stock_reservation USING btree (requested_by);


--
-- TOC entry 4601 (class 1259 OID 23786)
-- Name: idx_stock_transaction_receive_document_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_stock_transaction_receive_document_id ON public.stock_transaction USING btree (receive_document_id);


--
-- TOC entry 4602 (class 1259 OID 23460)
-- Name: idx_stock_transaction_return_document_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_stock_transaction_return_document_id ON public.stock_transaction USING btree (return_document_id);


--
-- TOC entry 4615 (class 1259 OID 22073)
-- Name: idx_stock_transfer_line_transfer; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_stock_transfer_line_transfer ON public.stock_transfer_line USING btree (transfer_id);


--
-- TOC entry 4603 (class 1259 OID 22074)
-- Name: idx_stock_txn_date; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_stock_txn_date ON public.stock_transaction USING btree (txn_date DESC);


--
-- TOC entry 4604 (class 1259 OID 22075)
-- Name: idx_stock_txn_item; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_stock_txn_item ON public.stock_transaction USING btree (item_id);


--
-- TOC entry 4605 (class 1259 OID 22076)
-- Name: idx_stock_txn_ref; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_stock_txn_ref ON public.stock_transaction USING btree (ref_doc_type, ref_doc_id);


--
-- TOC entry 4606 (class 1259 OID 22077)
-- Name: idx_stock_txn_type; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_stock_txn_type ON public.stock_transaction USING btree (txn_type);


--
-- TOC entry 4642 (class 1259 OID 22078)
-- Name: idx_users_dept_code; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_users_dept_code ON public.users USING btree (dept_code);


--
-- TOC entry 4659 (class 1259 OID 22082)
-- Name: idx_wo_cost_code_wo_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_wo_cost_code_wo_id ON public.work_order_cost_code USING btree (wo_id);


--
-- TOC entry 4667 (class 1259 OID 22079)
-- Name: idx_wo_payment_installment_wo_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_wo_payment_installment_wo_id ON public.work_order_payment_installment USING btree (wo_id);


--
-- TOC entry 4670 (class 1259 OID 22080)
-- Name: idx_wo_penalty_wo_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_wo_penalty_wo_id ON public.work_order_penalty USING btree (wo_id);


--
-- TOC entry 4673 (class 1259 OID 22081)
-- Name: idx_wo_retention_wo_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_wo_retention_wo_id ON public.work_order_retention USING btree (wo_id);


--
-- TOC entry 4721 (class 1259 OID 26472)
-- Name: idx_work_order_edit_log_wo; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_work_order_edit_log_wo ON public.work_order_edit_log USING btree (wo_id);


--
-- TOC entry 4664 (class 1259 OID 22083)
-- Name: idx_work_order_line_wo_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_work_order_line_wo_id ON public.work_order_line USING btree (wo_id);


--
-- TOC entry 4651 (class 1259 OID 22084)
-- Name: idx_work_order_project; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_work_order_project ON public.work_order USING btree (project_code);


--
-- TOC entry 4652 (class 1259 OID 22085)
-- Name: idx_work_order_status; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_work_order_status ON public.work_order USING btree (status);


--
-- TOC entry 4447 (class 1259 OID 23840)
-- Name: payment_log_reverses_uq; Type: INDEX; Schema: public; Owner: postgres
--

CREATE UNIQUE INDEX payment_log_reverses_uq ON public.payment_log USING btree (reverses_id) WHERE (reverses_id IS NOT NULL);


--
-- TOC entry 4489 (class 1259 OID 22086)
-- Name: project_customer_id_idx; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX project_customer_id_idx ON public.project USING btree (customer_id);


--
-- TOC entry 4351 (class 1259 OID 22087)
-- Name: uq_ic_po_receive_document_receive_no; Type: INDEX; Schema: public; Owner: postgres
--

CREATE UNIQUE INDEX uq_ic_po_receive_document_receive_no ON public.ic_po_receive_document USING btree (receive_no) WHERE (receive_no IS NOT NULL);


--
-- TOC entry 5015 (class 2620 OID 22088)
-- Name: role_menu_permissions trg_enforce_read; Type: TRIGGER; Schema: public; Owner: postgres
--

CREATE TRIGGER trg_enforce_read BEFORE INSERT OR UPDATE ON public.role_menu_permissions FOR EACH ROW EXECUTE FUNCTION public.enforce_read_before_write();


--
-- TOC entry 5017 (class 2620 OID 25873)
-- Name: project_cost_budget trg_project_cost_budget_item_only; Type: TRIGGER; Schema: public; Owner: postgres
--

CREATE TRIGGER trg_project_cost_budget_item_only BEFORE INSERT OR UPDATE OF cost_subgroup_id ON public.project_cost_budget FOR EACH ROW EXECUTE FUNCTION public.trg_project_cost_budget_item_only();


--
-- TOC entry 5016 (class 2620 OID 25844)
-- Name: project_job_share trg_project_job_share_total; Type: TRIGGER; Schema: public; Owner: postgres
--

CREATE CONSTRAINT TRIGGER trg_project_job_share_total AFTER INSERT OR UPDATE ON public.project_job_share DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION public.chk_project_job_share_total();


--
-- TOC entry 4724 (class 2606 OID 22089)
-- Name: approval_config approval_config_approver_role_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.approval_config
    ADD CONSTRAINT approval_config_approver_role_id_fkey FOREIGN KEY (approver_role_id) REFERENCES public.roles(id);


--
-- TOC entry 4725 (class 2606 OID 22094)
-- Name: approval_config approval_config_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.approval_config
    ADD CONSTRAINT approval_config_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- TOC entry 4726 (class 2606 OID 22099)
-- Name: approval_config approval_config_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.approval_config
    ADD CONSTRAINT approval_config_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- TOC entry 4727 (class 2606 OID 22104)
-- Name: approval_delegation approval_delegation_created_by_fk; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.approval_delegation
    ADD CONSTRAINT approval_delegation_created_by_fk FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- TOC entry 4728 (class 2606 OID 22109)
-- Name: approval_delegation approval_delegation_user_fk; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.approval_delegation
    ADD CONSTRAINT approval_delegation_user_fk FOREIGN KEY (user_id) REFERENCES public.users(id);


--
-- TOC entry 4729 (class 2606 OID 22114)
-- Name: approval_doc_types approval_doc_types_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.approval_doc_types
    ADD CONSTRAINT approval_doc_types_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- TOC entry 4730 (class 2606 OID 22119)
-- Name: approval_doc_types approval_doc_types_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.approval_doc_types
    ADD CONSTRAINT approval_doc_types_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- TOC entry 4731 (class 2606 OID 22124)
-- Name: approval_log approval_log_action_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.approval_log
    ADD CONSTRAINT approval_log_action_by_fkey FOREIGN KEY (action_by) REFERENCES public.users(id);


--
-- TOC entry 4732 (class 2606 OID 22129)
-- Name: approval_log approval_log_approval_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.approval_log
    ADD CONSTRAINT approval_log_approval_id_fkey FOREIGN KEY (approval_id) REFERENCES public.approval_request(id);


--
-- TOC entry 4733 (class 2606 OID 22134)
-- Name: approval_request approval_request_assigned_to_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.approval_request
    ADD CONSTRAINT approval_request_assigned_to_fkey FOREIGN KEY (assigned_to) REFERENCES public.users(id);


--
-- TOC entry 4734 (class 2606 OID 22139)
-- Name: approval_request approval_request_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.approval_request
    ADD CONSTRAINT approval_request_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- TOC entry 4735 (class 2606 OID 22144)
-- Name: approval_request approval_request_requested_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.approval_request
    ADD CONSTRAINT approval_request_requested_by_fkey FOREIGN KEY (requested_by) REFERENCES public.users(id);


--
-- TOC entry 4736 (class 2606 OID 22149)
-- Name: approval_request approval_request_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.approval_request
    ADD CONSTRAINT approval_request_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- TOC entry 4737 (class 2606 OID 22154)
-- Name: borrow borrow_approved_by_fk; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.borrow
    ADD CONSTRAINT borrow_approved_by_fk FOREIGN KEY (approved_by) REFERENCES public.users(id);


--
-- TOC entry 4738 (class 2606 OID 22159)
-- Name: borrow borrow_borrower_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.borrow
    ADD CONSTRAINT borrow_borrower_id_fkey FOREIGN KEY (borrower_id) REFERENCES public.users(id);


--
-- TOC entry 4739 (class 2606 OID 22164)
-- Name: borrow borrow_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.borrow
    ADD CONSTRAINT borrow_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- TOC entry 4743 (class 2606 OID 22169)
-- Name: borrow_line borrow_line_borrow_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.borrow_line
    ADD CONSTRAINT borrow_line_borrow_id_fkey FOREIGN KEY (borrow_id) REFERENCES public.borrow(id) ON DELETE CASCADE;


--
-- TOC entry 4744 (class 2606 OID 25905)
-- Name: borrow_line borrow_line_mat_code_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.borrow_line
    ADD CONSTRAINT borrow_line_mat_code_fkey FOREIGN KEY (mat_code) REFERENCES public.material_code(mat_code) DEFERRABLE;


--
-- TOC entry 4745 (class 2606 OID 22179)
-- Name: borrow_line borrow_line_stock_item_fk; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.borrow_line
    ADD CONSTRAINT borrow_line_stock_item_fk FOREIGN KEY (stock_item_id) REFERENCES public.stock_item(id) ON DELETE RESTRICT;


--
-- TOC entry 4740 (class 2606 OID 22184)
-- Name: borrow borrow_location_code_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.borrow
    ADD CONSTRAINT borrow_location_code_fkey FOREIGN KEY (location_code) REFERENCES public.location(location_code);


--
-- TOC entry 4746 (class 2606 OID 22189)
-- Name: borrow_status_log borrow_status_log_borrow_fk; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.borrow_status_log
    ADD CONSTRAINT borrow_status_log_borrow_fk FOREIGN KEY (borrow_id) REFERENCES public.borrow(id) ON DELETE CASCADE;


--
-- TOC entry 4747 (class 2606 OID 22194)
-- Name: borrow_status_log borrow_status_log_user_fk; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.borrow_status_log
    ADD CONSTRAINT borrow_status_log_user_fk FOREIGN KEY (changed_by) REFERENCES public.users(id);


--
-- TOC entry 4741 (class 2606 OID 22199)
-- Name: borrow borrow_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.borrow
    ADD CONSTRAINT borrow_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- TOC entry 4742 (class 2606 OID 22204)
-- Name: borrow borrow_warehouse_code_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.borrow
    ADD CONSTRAINT borrow_warehouse_code_fkey FOREIGN KEY (warehouse_code) REFERENCES public.warehouse(warehouse_code);


--
-- TOC entry 4748 (class 2606 OID 22209)
-- Name: brand brand_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.brand
    ADD CONSTRAINT brand_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- TOC entry 4749 (class 2606 OID 22214)
-- Name: brand brand_spec_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.brand
    ADD CONSTRAINT brand_spec_id_fkey FOREIGN KEY (spec_id) REFERENCES public.spec_size(id);


--
-- TOC entry 4750 (class 2606 OID 22219)
-- Name: brand brand_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.brand
    ADD CONSTRAINT brand_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- TOC entry 4751 (class 2606 OID 22224)
-- Name: cost_group cost_group_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.cost_group
    ADD CONSTRAINT cost_group_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- TOC entry 4752 (class 2606 OID 22229)
-- Name: cost_group cost_group_job_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.cost_group
    ADD CONSTRAINT cost_group_job_id_fkey FOREIGN KEY (job_id) REFERENCES public.cost_job(id);


--
-- TOC entry 4753 (class 2606 OID 22234)
-- Name: cost_group cost_group_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.cost_group
    ADD CONSTRAINT cost_group_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- TOC entry 4754 (class 2606 OID 22239)
-- Name: cost_job cost_job_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.cost_job
    ADD CONSTRAINT cost_job_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- TOC entry 4755 (class 2606 OID 22244)
-- Name: cost_job cost_job_subject_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.cost_job
    ADD CONSTRAINT cost_job_subject_id_fkey FOREIGN KEY (subject_id) REFERENCES public.cost_subject(id);


--
-- TOC entry 4756 (class 2606 OID 22249)
-- Name: cost_job cost_job_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.cost_job
    ADD CONSTRAINT cost_job_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- TOC entry 4757 (class 2606 OID 22254)
-- Name: cost_subgroup cost_subgroup_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.cost_subgroup
    ADD CONSTRAINT cost_subgroup_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- TOC entry 4758 (class 2606 OID 22259)
-- Name: cost_subgroup cost_subgroup_group_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.cost_subgroup
    ADD CONSTRAINT cost_subgroup_group_id_fkey FOREIGN KEY (group_id) REFERENCES public.cost_group(id);


--
-- TOC entry 4759 (class 2606 OID 22264)
-- Name: cost_subgroup cost_subgroup_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.cost_subgroup
    ADD CONSTRAINT cost_subgroup_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- TOC entry 4760 (class 2606 OID 22269)
-- Name: cost_subject cost_subject_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.cost_subject
    ADD CONSTRAINT cost_subject_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- TOC entry 4761 (class 2606 OID 22274)
-- Name: cost_subject cost_subject_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.cost_subject
    ADD CONSTRAINT cost_subject_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- TOC entry 4762 (class 2606 OID 25634)
-- Name: customer customer_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.customer
    ADD CONSTRAINT customer_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- TOC entry 4763 (class 2606 OID 22279)
-- Name: dept_menu_permissions dept_menu_permissions_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.dept_menu_permissions
    ADD CONSTRAINT dept_menu_permissions_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- TOC entry 4764 (class 2606 OID 25681)
-- Name: dept_menu_permissions dept_menu_permissions_dept_code_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.dept_menu_permissions
    ADD CONSTRAINT dept_menu_permissions_dept_code_fkey FOREIGN KEY (dept_code) REFERENCES public.departments(dept_code) ON UPDATE CASCADE;


--
-- TOC entry 4765 (class 2606 OID 22289)
-- Name: dept_menu_permissions dept_menu_permissions_menu_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.dept_menu_permissions
    ADD CONSTRAINT dept_menu_permissions_menu_id_fkey FOREIGN KEY (menu_id) REFERENCES public.menus(id) ON DELETE CASCADE;


--
-- TOC entry 4766 (class 2606 OID 22294)
-- Name: dept_menu_permissions dept_menu_permissions_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.dept_menu_permissions
    ADD CONSTRAINT dept_menu_permissions_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- TOC entry 4767 (class 2606 OID 22299)
-- Name: erp_audit_log erp_audit_log_changed_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.erp_audit_log
    ADD CONSTRAINT erp_audit_log_changed_by_fkey FOREIGN KEY (changed_by) REFERENCES public.users(id);


--
-- TOC entry 4768 (class 2606 OID 22304)
-- Name: grn fk_grn_supplier_id; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.grn
    ADD CONSTRAINT fk_grn_supplier_id FOREIGN KEY (supplier_id) REFERENCES public.supplier(id);


--
-- TOC entry 4858 (class 2606 OID 22309)
-- Name: petty_cash_requisition_line fk_pcl_project_code; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.petty_cash_requisition_line
    ADD CONSTRAINT fk_pcl_project_code FOREIGN KEY (project_code) REFERENCES public.project(project_code);


--
-- TOC entry 4859 (class 2606 OID 23788)
-- Name: petty_cash_requisition_line fk_pcl_supplier; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.petty_cash_requisition_line
    ADD CONSTRAINT fk_pcl_supplier FOREIGN KEY (supplier_id) REFERENCES public.supplier(id);


--
-- TOC entry 4860 (class 2606 OID 23793)
-- Name: petty_cash_requisition_line fk_pcl_wht_income_type; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.petty_cash_requisition_line
    ADD CONSTRAINT fk_pcl_wht_income_type FOREIGN KEY (wht_income_type_id) REFERENCES public.wht_income_type(id);


--
-- TOC entry 4879 (class 2606 OID 22314)
-- Name: purchase_order fk_po_supplier_id; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.purchase_order
    ADD CONSTRAINT fk_po_supplier_id FOREIGN KEY (supplier_id) REFERENCES public.supplier(id);


--
-- TOC entry 4872 (class 2606 OID 22319)
-- Name: pr_attachment fk_pr_attachment_pr; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.pr_attachment
    ADD CONSTRAINT fk_pr_attachment_pr FOREIGN KEY (pr_id) REFERENCES public.purchase_request(id);


--
-- TOC entry 4911 (class 2606 OID 22324)
-- Name: rfq fk_rfq_supplier_id; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.rfq
    ADD CONSTRAINT fk_rfq_supplier_id FOREIGN KEY (supplier_id) REFERENCES public.supplier(id);


--
-- TOC entry 4927 (class 2606 OID 25686)
-- Name: roles fk_roles_dept_code; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.roles
    ADD CONSTRAINT fk_roles_dept_code FOREIGN KEY (dept_code) REFERENCES public.departments(dept_code) ON UPDATE CASCADE;


--
-- TOC entry 4984 (class 2606 OID 25691)
-- Name: users fk_users_dept_code; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT fk_users_dept_code FOREIGN KEY (dept_code) REFERENCES public.departments(dept_code) ON UPDATE CASCADE;


--
-- TOC entry 4990 (class 2606 OID 22339)
-- Name: work_order fk_wo_supplier_id; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.work_order
    ADD CONSTRAINT fk_wo_supplier_id FOREIGN KEY (supplier_id) REFERENCES public.supplier(id);


--
-- TOC entry 4769 (class 2606 OID 22344)
-- Name: grn grn_confirmed_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.grn
    ADD CONSTRAINT grn_confirmed_by_fkey FOREIGN KEY (confirmed_by) REFERENCES public.users(id);


--
-- TOC entry 4770 (class 2606 OID 22349)
-- Name: grn grn_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.grn
    ADD CONSTRAINT grn_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- TOC entry 4775 (class 2606 OID 22354)
-- Name: grn_line grn_line_grn_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.grn_line
    ADD CONSTRAINT grn_line_grn_id_fkey FOREIGN KEY (grn_id) REFERENCES public.grn(id) ON DELETE CASCADE;


--
-- TOC entry 4776 (class 2606 OID 25910)
-- Name: grn_line grn_line_mat_code_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.grn_line
    ADD CONSTRAINT grn_line_mat_code_fkey FOREIGN KEY (mat_code) REFERENCES public.material_code(mat_code) DEFERRABLE;


--
-- TOC entry 4777 (class 2606 OID 22364)
-- Name: grn_line grn_line_po_line_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.grn_line
    ADD CONSTRAINT grn_line_po_line_id_fkey FOREIGN KEY (po_line_id) REFERENCES public.purchase_order_line(id);


--
-- TOC entry 4778 (class 2606 OID 22369)
-- Name: grn_line grn_line_zone_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.grn_line
    ADD CONSTRAINT grn_line_zone_id_fkey FOREIGN KEY (zone_id) REFERENCES public.storage_zone(id);


--
-- TOC entry 4771 (class 2606 OID 22374)
-- Name: grn grn_po_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.grn
    ADD CONSTRAINT grn_po_id_fkey FOREIGN KEY (po_id) REFERENCES public.purchase_order(id);


--
-- TOC entry 4772 (class 2606 OID 22379)
-- Name: grn grn_received_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.grn
    ADD CONSTRAINT grn_received_by_fkey FOREIGN KEY (received_by) REFERENCES public.users(id);


--
-- TOC entry 4773 (class 2606 OID 22384)
-- Name: grn grn_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.grn
    ADD CONSTRAINT grn_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- TOC entry 4774 (class 2606 OID 22389)
-- Name: grn grn_warehouse_code_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.grn
    ADD CONSTRAINT grn_warehouse_code_fkey FOREIGN KEY (warehouse_code) REFERENCES public.warehouse(warehouse_code);


--
-- TOC entry 4779 (class 2606 OID 22394)
-- Name: ic_po_receive_document ic_po_receive_document_po_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.ic_po_receive_document
    ADD CONSTRAINT ic_po_receive_document_po_id_fkey FOREIGN KEY (po_id) REFERENCES public.purchase_order(id);


--
-- TOC entry 4999 (class 2606 OID 23416)
-- Name: ic_po_return_document ic_po_return_document_po_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.ic_po_return_document
    ADD CONSTRAINT ic_po_return_document_po_id_fkey FOREIGN KEY (po_id) REFERENCES public.purchase_order(id);


--
-- TOC entry 5000 (class 2606 OID 23444)
-- Name: ic_po_return_line ic_po_return_line_cost_subgroup_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.ic_po_return_line
    ADD CONSTRAINT ic_po_return_line_cost_subgroup_id_fkey FOREIGN KEY (cost_subgroup_id) REFERENCES public.cost_subgroup(id);


--
-- TOC entry 5001 (class 2606 OID 23439)
-- Name: ic_po_return_line ic_po_return_line_po_line_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.ic_po_return_line
    ADD CONSTRAINT ic_po_return_line_po_line_id_fkey FOREIGN KEY (po_line_id) REFERENCES public.purchase_order_line(id);


--
-- TOC entry 5002 (class 2606 OID 23434)
-- Name: ic_po_return_line ic_po_return_line_return_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.ic_po_return_line
    ADD CONSTRAINT ic_po_return_line_return_id_fkey FOREIGN KEY (return_id) REFERENCES public.ic_po_return_document(id);


--
-- TOC entry 4780 (class 2606 OID 22399)
-- Name: ic_project_cost_item ic_project_cost_item_cost_subgroup_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.ic_project_cost_item
    ADD CONSTRAINT ic_project_cost_item_cost_subgroup_id_fkey FOREIGN KEY (cost_subgroup_id) REFERENCES public.cost_subgroup(id);


--
-- TOC entry 4781 (class 2606 OID 22404)
-- Name: ic_project_cost_item_transaction ic_project_cost_item_transaction_movement_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.ic_project_cost_item_transaction
    ADD CONSTRAINT ic_project_cost_item_transaction_movement_id_fkey FOREIGN KEY (movement_id) REFERENCES public.ic_project_movement(id);


--
-- TOC entry 4782 (class 2606 OID 22409)
-- Name: ic_project_cost_item_transaction ic_project_cost_item_transaction_movement_line_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.ic_project_cost_item_transaction
    ADD CONSTRAINT ic_project_cost_item_transaction_movement_line_id_fkey FOREIGN KEY (movement_line_id) REFERENCES public.ic_project_movement_line(id);


--
-- TOC entry 4783 (class 2606 OID 22414)
-- Name: ic_project_cost_item_transaction ic_project_cost_item_transaction_po_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.ic_project_cost_item_transaction
    ADD CONSTRAINT ic_project_cost_item_transaction_po_id_fkey FOREIGN KEY (po_id) REFERENCES public.purchase_order(id);


--
-- TOC entry 4784 (class 2606 OID 22419)
-- Name: ic_project_cost_item_transaction ic_project_cost_item_transaction_po_line_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.ic_project_cost_item_transaction
    ADD CONSTRAINT ic_project_cost_item_transaction_po_line_id_fkey FOREIGN KEY (po_line_id) REFERENCES public.purchase_order_line(id);


--
-- TOC entry 4785 (class 2606 OID 23657)
-- Name: ic_project_cost_item_transaction ic_project_cost_item_transaction_pr_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.ic_project_cost_item_transaction
    ADD CONSTRAINT ic_project_cost_item_transaction_pr_id_fkey FOREIGN KEY (pr_id) REFERENCES public.purchase_request(id);


--
-- TOC entry 4786 (class 2606 OID 23662)
-- Name: ic_project_cost_item_transaction ic_project_cost_item_transaction_pr_line_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.ic_project_cost_item_transaction
    ADD CONSTRAINT ic_project_cost_item_transaction_pr_line_id_fkey FOREIGN KEY (pr_line_id) REFERENCES public.purchase_request_line(id);


--
-- TOC entry 4787 (class 2606 OID 22424)
-- Name: ic_project_cost_item_transaction ic_project_cost_item_transaction_project_cost_item_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.ic_project_cost_item_transaction
    ADD CONSTRAINT ic_project_cost_item_transaction_project_cost_item_id_fkey FOREIGN KEY (project_cost_item_id) REFERENCES public.ic_project_cost_item(id);


--
-- TOC entry 4788 (class 2606 OID 22429)
-- Name: ic_project_cost_item_transaction ic_project_cost_item_transaction_receive_document_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.ic_project_cost_item_transaction
    ADD CONSTRAINT ic_project_cost_item_transaction_receive_document_id_fkey FOREIGN KEY (receive_document_id) REFERENCES public.ic_po_receive_document(id);


--
-- TOC entry 4789 (class 2606 OID 23455)
-- Name: ic_project_cost_item_transaction ic_project_cost_item_transaction_return_document_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.ic_project_cost_item_transaction
    ADD CONSTRAINT ic_project_cost_item_transaction_return_document_id_fkey FOREIGN KEY (return_document_id) REFERENCES public.ic_po_return_document(id);


--
-- TOC entry 4792 (class 2606 OID 22434)
-- Name: ic_project_movement_line ic_project_movement_line_cost_subgroup_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.ic_project_movement_line
    ADD CONSTRAINT ic_project_movement_line_cost_subgroup_id_fkey FOREIGN KEY (cost_subgroup_id) REFERENCES public.cost_subgroup(id);


--
-- TOC entry 4793 (class 2606 OID 22439)
-- Name: ic_project_movement_line ic_project_movement_line_mat_code_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.ic_project_movement_line
    ADD CONSTRAINT ic_project_movement_line_mat_code_fkey FOREIGN KEY (mat_code) REFERENCES public.material_code(mat_code);


--
-- TOC entry 4794 (class 2606 OID 22444)
-- Name: ic_project_movement_line ic_project_movement_line_movement_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.ic_project_movement_line
    ADD CONSTRAINT ic_project_movement_line_movement_id_fkey FOREIGN KEY (movement_id) REFERENCES public.ic_project_movement(id);


--
-- TOC entry 4795 (class 2606 OID 22449)
-- Name: ic_project_movement_line ic_project_movement_line_to_cost_subgroup_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.ic_project_movement_line
    ADD CONSTRAINT ic_project_movement_line_to_cost_subgroup_id_fkey FOREIGN KEY (to_cost_subgroup_id) REFERENCES public.cost_subgroup(id);


--
-- TOC entry 4796 (class 2606 OID 22454)
-- Name: ic_project_movement_line ic_project_movement_line_to_project_code_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.ic_project_movement_line
    ADD CONSTRAINT ic_project_movement_line_to_project_code_fkey FOREIGN KEY (to_project_code) REFERENCES public.project(project_code);


--
-- TOC entry 4790 (class 2606 OID 22459)
-- Name: ic_project_movement ic_project_movement_project_code_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.ic_project_movement
    ADD CONSTRAINT ic_project_movement_project_code_fkey FOREIGN KEY (project_code) REFERENCES public.project(project_code);


--
-- TOC entry 4791 (class 2606 OID 22464)
-- Name: ic_project_movement ic_project_movement_requested_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.ic_project_movement
    ADD CONSTRAINT ic_project_movement_requested_by_fkey FOREIGN KEY (requested_by) REFERENCES public.users(id);


--
-- TOC entry 4797 (class 2606 OID 22469)
-- Name: ic_project_receipt_pool ic_project_receipt_pool_cost_subgroup_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.ic_project_receipt_pool
    ADD CONSTRAINT ic_project_receipt_pool_cost_subgroup_id_fkey FOREIGN KEY (cost_subgroup_id) REFERENCES public.cost_subgroup(id);


--
-- TOC entry 4798 (class 2606 OID 22474)
-- Name: ic_project_receipt_pool ic_project_receipt_pool_mat_code_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.ic_project_receipt_pool
    ADD CONSTRAINT ic_project_receipt_pool_mat_code_fkey FOREIGN KEY (mat_code) REFERENCES public.material_code(mat_code);


--
-- TOC entry 4799 (class 2606 OID 22479)
-- Name: ic_project_receipt_pool ic_project_receipt_pool_project_code_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.ic_project_receipt_pool
    ADD CONSTRAINT ic_project_receipt_pool_project_code_fkey FOREIGN KEY (project_code) REFERENCES public.project(project_code);


--
-- TOC entry 4800 (class 2606 OID 22484)
-- Name: ic_project_receipt_pool_transaction ic_project_receipt_pool_transaction_movement_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.ic_project_receipt_pool_transaction
    ADD CONSTRAINT ic_project_receipt_pool_transaction_movement_id_fkey FOREIGN KEY (movement_id) REFERENCES public.ic_project_movement(id);


--
-- TOC entry 4801 (class 2606 OID 22489)
-- Name: ic_project_receipt_pool_transaction ic_project_receipt_pool_transaction_movement_line_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.ic_project_receipt_pool_transaction
    ADD CONSTRAINT ic_project_receipt_pool_transaction_movement_line_id_fkey FOREIGN KEY (movement_line_id) REFERENCES public.ic_project_movement_line(id);


--
-- TOC entry 4802 (class 2606 OID 22494)
-- Name: ic_project_receipt_pool_transaction ic_project_receipt_pool_transaction_po_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.ic_project_receipt_pool_transaction
    ADD CONSTRAINT ic_project_receipt_pool_transaction_po_id_fkey FOREIGN KEY (po_id) REFERENCES public.purchase_order(id);


--
-- TOC entry 4803 (class 2606 OID 22499)
-- Name: ic_project_receipt_pool_transaction ic_project_receipt_pool_transaction_po_line_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.ic_project_receipt_pool_transaction
    ADD CONSTRAINT ic_project_receipt_pool_transaction_po_line_id_fkey FOREIGN KEY (po_line_id) REFERENCES public.purchase_order_line(id);


--
-- TOC entry 4804 (class 2606 OID 22504)
-- Name: ic_project_receipt_pool_transaction ic_project_receipt_pool_transaction_receipt_pool_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.ic_project_receipt_pool_transaction
    ADD CONSTRAINT ic_project_receipt_pool_transaction_receipt_pool_id_fkey FOREIGN KEY (receipt_pool_id) REFERENCES public.ic_project_receipt_pool(id);


--
-- TOC entry 4805 (class 2606 OID 22509)
-- Name: ic_project_receipt_pool_transaction ic_project_receipt_pool_transaction_receive_document_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.ic_project_receipt_pool_transaction
    ADD CONSTRAINT ic_project_receipt_pool_transaction_receive_document_id_fkey FOREIGN KEY (receive_document_id) REFERENCES public.ic_po_receive_document(id);


--
-- TOC entry 4806 (class 2606 OID 22514)
-- Name: inventory inventory_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.inventory
    ADD CONSTRAINT inventory_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- TOC entry 4807 (class 2606 OID 25915)
-- Name: inventory inventory_mat_code_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.inventory
    ADD CONSTRAINT inventory_mat_code_fkey FOREIGN KEY (mat_code) REFERENCES public.material_code(mat_code) DEFERRABLE;


--
-- TOC entry 4811 (class 2606 OID 22524)
-- Name: inventory_transaction inventory_transaction_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.inventory_transaction
    ADD CONSTRAINT inventory_transaction_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- TOC entry 4812 (class 2606 OID 22529)
-- Name: inventory_transaction inventory_transaction_from_warehouse_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.inventory_transaction
    ADD CONSTRAINT inventory_transaction_from_warehouse_fkey FOREIGN KEY (from_warehouse) REFERENCES public.warehouse(warehouse_code);


--
-- TOC entry 4813 (class 2606 OID 22534)
-- Name: inventory_transaction inventory_transaction_from_zone_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.inventory_transaction
    ADD CONSTRAINT inventory_transaction_from_zone_id_fkey FOREIGN KEY (from_zone_id) REFERENCES public.storage_zone(id);


--
-- TOC entry 4814 (class 2606 OID 22539)
-- Name: inventory_transaction inventory_transaction_location_code_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.inventory_transaction
    ADD CONSTRAINT inventory_transaction_location_code_fkey FOREIGN KEY (location_code) REFERENCES public.location(location_code);


--
-- TOC entry 4815 (class 2606 OID 25920)
-- Name: inventory_transaction inventory_transaction_mat_code_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.inventory_transaction
    ADD CONSTRAINT inventory_transaction_mat_code_fkey FOREIGN KEY (mat_code) REFERENCES public.material_code(mat_code) DEFERRABLE;


--
-- TOC entry 4816 (class 2606 OID 22549)
-- Name: inventory_transaction inventory_transaction_to_warehouse_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.inventory_transaction
    ADD CONSTRAINT inventory_transaction_to_warehouse_fkey FOREIGN KEY (to_warehouse) REFERENCES public.warehouse(warehouse_code);


--
-- TOC entry 4817 (class 2606 OID 22554)
-- Name: inventory_transaction inventory_transaction_to_zone_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.inventory_transaction
    ADD CONSTRAINT inventory_transaction_to_zone_id_fkey FOREIGN KEY (to_zone_id) REFERENCES public.storage_zone(id);


--
-- TOC entry 4818 (class 2606 OID 22559)
-- Name: inventory_transaction inventory_transaction_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.inventory_transaction
    ADD CONSTRAINT inventory_transaction_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- TOC entry 4808 (class 2606 OID 22564)
-- Name: inventory inventory_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.inventory
    ADD CONSTRAINT inventory_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- TOC entry 4809 (class 2606 OID 22569)
-- Name: inventory inventory_warehouse_code_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.inventory
    ADD CONSTRAINT inventory_warehouse_code_fkey FOREIGN KEY (warehouse_code) REFERENCES public.warehouse(warehouse_code);


--
-- TOC entry 4810 (class 2606 OID 22574)
-- Name: inventory inventory_zone_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.inventory
    ADD CONSTRAINT inventory_zone_id_fkey FOREIGN KEY (zone_id) REFERENCES public.storage_zone(id);


--
-- TOC entry 4819 (class 2606 OID 22579)
-- Name: location location_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.location
    ADD CONSTRAINT location_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- TOC entry 4820 (class 2606 OID 22584)
-- Name: location location_parent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.location
    ADD CONSTRAINT location_parent_id_fkey FOREIGN KEY (parent_id) REFERENCES public.location(id);


--
-- TOC entry 4821 (class 2606 OID 22589)
-- Name: location location_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.location
    ADD CONSTRAINT location_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- TOC entry 4822 (class 2606 OID 22594)
-- Name: location location_warehouse_code_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.location
    ADD CONSTRAINT location_warehouse_code_fkey FOREIGN KEY (warehouse_code) REFERENCES public.warehouse(warehouse_code);


--
-- TOC entry 4823 (class 2606 OID 22599)
-- Name: mat_group mat_group_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.mat_group
    ADD CONSTRAINT mat_group_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- TOC entry 4824 (class 2606 OID 22604)
-- Name: mat_group mat_group_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.mat_group
    ADD CONSTRAINT mat_group_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- TOC entry 4825 (class 2606 OID 22609)
-- Name: mat_name mat_name_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.mat_name
    ADD CONSTRAINT mat_name_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- TOC entry 4826 (class 2606 OID 22614)
-- Name: mat_name mat_name_subgroup_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.mat_name
    ADD CONSTRAINT mat_name_subgroup_id_fkey FOREIGN KEY (subgroup_id) REFERENCES public.subgroup(id);


--
-- TOC entry 4827 (class 2606 OID 22619)
-- Name: mat_name mat_name_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.mat_name
    ADD CONSTRAINT mat_name_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- TOC entry 4828 (class 2606 OID 22624)
-- Name: material_code material_code_brand_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.material_code
    ADD CONSTRAINT material_code_brand_id_fkey FOREIGN KEY (brand_id) REFERENCES public.brand(id);


--
-- TOC entry 4829 (class 2606 OID 22629)
-- Name: material_code material_code_cost_subgroup_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.material_code
    ADD CONSTRAINT material_code_cost_subgroup_id_fkey FOREIGN KEY (cost_subgroup_id) REFERENCES public.cost_subgroup(id);


--
-- TOC entry 4830 (class 2606 OID 22634)
-- Name: material_code material_code_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.material_code
    ADD CONSTRAINT material_code_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- TOC entry 4831 (class 2606 OID 22639)
-- Name: material_code material_code_group_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.material_code
    ADD CONSTRAINT material_code_group_id_fkey FOREIGN KEY (group_id) REFERENCES public.mat_group(id);


--
-- TOC entry 4832 (class 2606 OID 22644)
-- Name: material_code material_code_mat_name_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.material_code
    ADD CONSTRAINT material_code_mat_name_id_fkey FOREIGN KEY (mat_name_id) REFERENCES public.mat_name(id);


--
-- TOC entry 4833 (class 2606 OID 22649)
-- Name: material_code material_code_spec_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.material_code
    ADD CONSTRAINT material_code_spec_id_fkey FOREIGN KEY (spec_id) REFERENCES public.spec_size(id);


--
-- TOC entry 4834 (class 2606 OID 22654)
-- Name: material_code material_code_subgroup_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.material_code
    ADD CONSTRAINT material_code_subgroup_id_fkey FOREIGN KEY (subgroup_id) REFERENCES public.subgroup(id);


--
-- TOC entry 4835 (class 2606 OID 22659)
-- Name: material_code material_code_unit_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.material_code
    ADD CONSTRAINT material_code_unit_id_fkey FOREIGN KEY (unit_id) REFERENCES public.unit(id);


--
-- TOC entry 4836 (class 2606 OID 22664)
-- Name: material_code material_code_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.material_code
    ADD CONSTRAINT material_code_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- TOC entry 4837 (class 2606 OID 22669)
-- Name: memo memo_approver_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.memo
    ADD CONSTRAINT memo_approver_id_fk FOREIGN KEY (approver_id) REFERENCES public.users(id);


--
-- TOC entry 4838 (class 2606 OID 22674)
-- Name: memo_attachment memo_attachment_memo_fk; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.memo_attachment
    ADD CONSTRAINT memo_attachment_memo_fk FOREIGN KEY (memo_id) REFERENCES public.memo(id) ON DELETE CASCADE;


--
-- TOC entry 4839 (class 2606 OID 22679)
-- Name: memo_attachment memo_attachment_user_fk; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.memo_attachment
    ADD CONSTRAINT memo_attachment_user_fk FOREIGN KEY (uploaded_by) REFERENCES public.users(id);


--
-- TOC entry 4840 (class 2606 OID 22684)
-- Name: memo_line memo_line_memo_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.memo_line
    ADD CONSTRAINT memo_line_memo_id_fk FOREIGN KEY (memo_id) REFERENCES public.memo(id) ON DELETE CASCADE;


--
-- TOC entry 4841 (class 2606 OID 22689)
-- Name: memo_status_log memo_status_log_memo_fk; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.memo_status_log
    ADD CONSTRAINT memo_status_log_memo_fk FOREIGN KEY (memo_id) REFERENCES public.memo(id) ON DELETE CASCADE;


--
-- TOC entry 4842 (class 2606 OID 22694)
-- Name: memo_status_log memo_status_log_user_fk; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.memo_status_log
    ADD CONSTRAINT memo_status_log_user_fk FOREIGN KEY (changed_by) REFERENCES public.users(id);


--
-- TOC entry 4843 (class 2606 OID 22699)
-- Name: menus menus_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.menus
    ADD CONSTRAINT menus_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- TOC entry 4844 (class 2606 OID 22704)
-- Name: menus menus_parent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.menus
    ADD CONSTRAINT menus_parent_id_fkey FOREIGN KEY (parent_id) REFERENCES public.menus(id) ON DELETE CASCADE;


--
-- TOC entry 4845 (class 2606 OID 22709)
-- Name: menus menus_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.menus
    ADD CONSTRAINT menus_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- TOC entry 4846 (class 2606 OID 22714)
-- Name: modules modules_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.modules
    ADD CONSTRAINT modules_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- TOC entry 4847 (class 2606 OID 22719)
-- Name: modules modules_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.modules
    ADD CONSTRAINT modules_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- TOC entry 5005 (class 2606 OID 25593)
-- Name: payment_installment payment_installment_paid_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.payment_installment
    ADD CONSTRAINT payment_installment_paid_by_fkey FOREIGN KEY (paid_by) REFERENCES public.users(id);


--
-- TOC entry 5006 (class 2606 OID 25628)
-- Name: payment_installment payment_installment_payment_log_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.payment_installment
    ADD CONSTRAINT payment_installment_payment_log_id_fkey FOREIGN KEY (payment_log_id) REFERENCES public.payment_log(id);


--
-- TOC entry 4848 (class 2606 OID 22724)
-- Name: payment_log payment_log_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.payment_log
    ADD CONSTRAINT payment_log_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- TOC entry 4849 (class 2606 OID 22729)
-- Name: payment_log payment_log_paid_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.payment_log
    ADD CONSTRAINT payment_log_paid_by_fkey FOREIGN KEY (paid_by) REFERENCES public.users(id);


--
-- TOC entry 4850 (class 2606 OID 22734)
-- Name: payment_log payment_log_reverses_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.payment_log
    ADD CONSTRAINT payment_log_reverses_id_fkey FOREIGN KEY (reverses_id) REFERENCES public.payment_log(id);


--
-- TOC entry 4851 (class 2606 OID 22739)
-- Name: permission_audit_logs permission_audit_logs_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.permission_audit_logs
    ADD CONSTRAINT permission_audit_logs_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id);


--
-- TOC entry 4852 (class 2606 OID 22744)
-- Name: permissions permissions_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.permissions
    ADD CONSTRAINT permissions_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- TOC entry 4853 (class 2606 OID 22749)
-- Name: permissions permissions_module_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.permissions
    ADD CONSTRAINT permissions_module_id_fkey FOREIGN KEY (module_id) REFERENCES public.modules(id) ON DELETE CASCADE;


--
-- TOC entry 4854 (class 2606 OID 22754)
-- Name: permissions permissions_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.permissions
    ADD CONSTRAINT permissions_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- TOC entry 5003 (class 2606 OID 23695)
-- Name: petty_cash_attachment petty_cash_attachment_pc_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.petty_cash_attachment
    ADD CONSTRAINT petty_cash_attachment_pc_id_fkey FOREIGN KEY (pc_id) REFERENCES public.petty_cash_requisition(id) ON DELETE CASCADE;


--
-- TOC entry 5004 (class 2606 OID 23700)
-- Name: petty_cash_attachment petty_cash_attachment_uploaded_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.petty_cash_attachment
    ADD CONSTRAINT petty_cash_attachment_uploaded_by_fkey FOREIGN KEY (uploaded_by) REFERENCES public.users(id);


--
-- TOC entry 4855 (class 2606 OID 22759)
-- Name: petty_cash_requisition petty_cash_requisition_approver_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.petty_cash_requisition
    ADD CONSTRAINT petty_cash_requisition_approver_id_fkey FOREIGN KEY (approver_id) REFERENCES public.users(id);


--
-- TOC entry 4861 (class 2606 OID 22764)
-- Name: petty_cash_requisition_line petty_cash_requisition_line_cost_subgroup_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.petty_cash_requisition_line
    ADD CONSTRAINT petty_cash_requisition_line_cost_subgroup_id_fkey FOREIGN KEY (cost_subgroup_id) REFERENCES public.cost_subgroup(id);


--
-- TOC entry 4862 (class 2606 OID 22769)
-- Name: petty_cash_requisition_line petty_cash_requisition_line_mat_code_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.petty_cash_requisition_line
    ADD CONSTRAINT petty_cash_requisition_line_mat_code_fkey FOREIGN KEY (mat_code) REFERENCES public.material_code(mat_code);


--
-- TOC entry 4863 (class 2606 OID 22774)
-- Name: petty_cash_requisition_line petty_cash_requisition_line_pc_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.petty_cash_requisition_line
    ADD CONSTRAINT petty_cash_requisition_line_pc_id_fkey FOREIGN KEY (pc_id) REFERENCES public.petty_cash_requisition(id) ON DELETE CASCADE;


--
-- TOC entry 4856 (class 2606 OID 23675)
-- Name: petty_cash_requisition petty_cash_requisition_project_code_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.petty_cash_requisition
    ADD CONSTRAINT petty_cash_requisition_project_code_fkey FOREIGN KEY (project_code) REFERENCES public.project(project_code);


--
-- TOC entry 4857 (class 2606 OID 22779)
-- Name: petty_cash_requisition petty_cash_requisition_requested_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.petty_cash_requisition
    ADD CONSTRAINT petty_cash_requisition_requested_by_fkey FOREIGN KEY (requested_by) REFERENCES public.users(id);


--
-- TOC entry 4864 (class 2606 OID 22784)
-- Name: petty_cash_status_log petty_cash_status_log_changed_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.petty_cash_status_log
    ADD CONSTRAINT petty_cash_status_log_changed_by_fkey FOREIGN KEY (changed_by) REFERENCES public.users(id);


--
-- TOC entry 4865 (class 2606 OID 22789)
-- Name: petty_cash_status_log petty_cash_status_log_pc_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.petty_cash_status_log
    ADD CONSTRAINT petty_cash_status_log_pc_id_fkey FOREIGN KEY (pc_id) REFERENCES public.petty_cash_requisition(id) ON DELETE CASCADE;


--
-- TOC entry 4866 (class 2606 OID 22794)
-- Name: po_attachment po_attachment_po_fk; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.po_attachment
    ADD CONSTRAINT po_attachment_po_fk FOREIGN KEY (po_id) REFERENCES public.purchase_order(id);


--
-- TOC entry 4867 (class 2606 OID 22799)
-- Name: po_attachment po_attachment_uploaded_by_fk; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.po_attachment
    ADD CONSTRAINT po_attachment_uploaded_by_fk FOREIGN KEY (uploaded_by) REFERENCES public.users(id);


--
-- TOC entry 4868 (class 2606 OID 22804)
-- Name: po_edit_log po_edit_log_po_fk; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.po_edit_log
    ADD CONSTRAINT po_edit_log_po_fk FOREIGN KEY (po_id) REFERENCES public.purchase_order(id);


--
-- TOC entry 4869 (class 2606 OID 22809)
-- Name: po_edit_log po_edit_log_user_fk; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.po_edit_log
    ADD CONSTRAINT po_edit_log_user_fk FOREIGN KEY (edited_by) REFERENCES public.users(id);


--
-- TOC entry 4870 (class 2606 OID 22814)
-- Name: po_status_log po_status_log_changed_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.po_status_log
    ADD CONSTRAINT po_status_log_changed_by_fkey FOREIGN KEY (changed_by) REFERENCES public.users(id);


--
-- TOC entry 4871 (class 2606 OID 22819)
-- Name: po_status_log po_status_log_po_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.po_status_log
    ADD CONSTRAINT po_status_log_po_id_fkey FOREIGN KEY (po_id) REFERENCES public.purchase_order(id);


--
-- TOC entry 4894 (class 2606 OID 22824)
-- Name: purchase_request pr_memo_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.purchase_request
    ADD CONSTRAINT pr_memo_id_fk FOREIGN KEY (memo_id) REFERENCES public.memo(id) ON DELETE SET NULL;


--
-- TOC entry 4873 (class 2606 OID 22829)
-- Name: pr_status_log pr_status_log_changed_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.pr_status_log
    ADD CONSTRAINT pr_status_log_changed_by_fkey FOREIGN KEY (changed_by) REFERENCES public.users(id);


--
-- TOC entry 4874 (class 2606 OID 22834)
-- Name: pr_status_log pr_status_log_pr_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.pr_status_log
    ADD CONSTRAINT pr_status_log_pr_id_fkey FOREIGN KEY (pr_id) REFERENCES public.purchase_request(id);


--
-- TOC entry 5007 (class 2606 OID 25654)
-- Name: project_code_counter project_code_counter_cus_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.project_code_counter
    ADD CONSTRAINT project_code_counter_cus_id_fkey FOREIGN KEY (cus_id) REFERENCES public.customer(cus_id);


--
-- TOC entry 5011 (class 2606 OID 25866)
-- Name: project_cost_budget project_cost_budget_cost_subgroup_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.project_cost_budget
    ADD CONSTRAINT project_cost_budget_cost_subgroup_id_fkey FOREIGN KEY (cost_subgroup_id) REFERENCES public.cost_subgroup(id);


--
-- TOC entry 5012 (class 2606 OID 25861)
-- Name: project_cost_budget project_cost_budget_project_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.project_cost_budget
    ADD CONSTRAINT project_cost_budget_project_id_fkey FOREIGN KEY (project_id) REFERENCES public.project(id) ON DELETE CASCADE;


--
-- TOC entry 4875 (class 2606 OID 22839)
-- Name: project project_customer_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.project
    ADD CONSTRAINT project_customer_id_fkey FOREIGN KEY (customer_id) REFERENCES public.customer(cus_id);


--
-- TOC entry 4876 (class 2606 OID 25696)
-- Name: project project_dept_code_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.project
    ADD CONSTRAINT project_dept_code_fkey FOREIGN KEY (dept_code) REFERENCES public.departments(dept_code) ON UPDATE CASCADE;


--
-- TOC entry 5010 (class 2606 OID 25816)
-- Name: project_job_share project_job_share_project_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.project_job_share
    ADD CONSTRAINT project_job_share_project_id_fkey FOREIGN KEY (project_id) REFERENCES public.project(id) ON DELETE CASCADE;


--
-- TOC entry 4877 (class 2606 OID 22849)
-- Name: project project_linked_warehouse_code_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.project
    ADD CONSTRAINT project_linked_warehouse_code_fkey FOREIGN KEY (linked_warehouse_code) REFERENCES public.warehouse(warehouse_code);


--
-- TOC entry 4878 (class 2606 OID 22854)
-- Name: project project_owner_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.project
    ADD CONSTRAINT project_owner_id_fk FOREIGN KEY (owner_id) REFERENCES public.users(id);


--
-- TOC entry 5008 (class 2606 OID 25774)
-- Name: project_responsible_person project_responsible_person_project_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.project_responsible_person
    ADD CONSTRAINT project_responsible_person_project_id_fkey FOREIGN KEY (project_id) REFERENCES public.project(id) ON DELETE CASCADE;


--
-- TOC entry 5009 (class 2606 OID 25779)
-- Name: project_responsible_person project_responsible_person_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.project_responsible_person
    ADD CONSTRAINT project_responsible_person_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id);


--
-- TOC entry 4880 (class 2606 OID 22859)
-- Name: purchase_order purchase_order_approver_fk; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.purchase_order
    ADD CONSTRAINT purchase_order_approver_fk FOREIGN KEY (approver_id) REFERENCES public.users(id);


--
-- TOC entry 4881 (class 2606 OID 22864)
-- Name: purchase_order purchase_order_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.purchase_order
    ADD CONSTRAINT purchase_order_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- TOC entry 4882 (class 2606 OID 25565)
-- Name: purchase_order purchase_order_deposit_paid_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.purchase_order
    ADD CONSTRAINT purchase_order_deposit_paid_by_fkey FOREIGN KEY (deposit_paid_by) REFERENCES public.users(id);


--
-- TOC entry 4890 (class 2606 OID 22869)
-- Name: purchase_order_line purchase_order_line_cost_subgroup_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.purchase_order_line
    ADD CONSTRAINT purchase_order_line_cost_subgroup_id_fkey FOREIGN KEY (cost_subgroup_id) REFERENCES public.cost_subgroup(id);


--
-- TOC entry 4891 (class 2606 OID 25925)
-- Name: purchase_order_line purchase_order_line_mat_code_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.purchase_order_line
    ADD CONSTRAINT purchase_order_line_mat_code_fkey FOREIGN KEY (mat_code) REFERENCES public.material_code(mat_code) DEFERRABLE;


--
-- TOC entry 4892 (class 2606 OID 22879)
-- Name: purchase_order_line purchase_order_line_po_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.purchase_order_line
    ADD CONSTRAINT purchase_order_line_po_id_fkey FOREIGN KEY (po_id) REFERENCES public.purchase_order(id) ON DELETE CASCADE;


--
-- TOC entry 4893 (class 2606 OID 22884)
-- Name: purchase_order_line purchase_order_line_pr_line_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.purchase_order_line
    ADD CONSTRAINT purchase_order_line_pr_line_id_fkey FOREIGN KEY (pr_line_id) REFERENCES public.purchase_request_line(id);


--
-- TOC entry 4883 (class 2606 OID 22889)
-- Name: purchase_order purchase_order_location_fk; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.purchase_order
    ADD CONSTRAINT purchase_order_location_fk FOREIGN KEY (location_code) REFERENCES public.location(location_code);


--
-- TOC entry 4884 (class 2606 OID 22894)
-- Name: purchase_order purchase_order_pr_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.purchase_order
    ADD CONSTRAINT purchase_order_pr_id_fkey FOREIGN KEY (pr_id) REFERENCES public.purchase_request(id);


--
-- TOC entry 4885 (class 2606 OID 22899)
-- Name: purchase_order purchase_order_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.purchase_order
    ADD CONSTRAINT purchase_order_project_fk FOREIGN KEY (project_code) REFERENCES public.project(project_code);


--
-- TOC entry 4886 (class 2606 OID 22904)
-- Name: purchase_order purchase_order_requested_by_fk; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.purchase_order
    ADD CONSTRAINT purchase_order_requested_by_fk FOREIGN KEY (requested_by) REFERENCES public.users(id);


--
-- TOC entry 4887 (class 2606 OID 22909)
-- Name: purchase_order purchase_order_rfq_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.purchase_order
    ADD CONSTRAINT purchase_order_rfq_id_fkey FOREIGN KEY (rfq_id) REFERENCES public.rfq(id);


--
-- TOC entry 4888 (class 2606 OID 22914)
-- Name: purchase_order purchase_order_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.purchase_order
    ADD CONSTRAINT purchase_order_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- TOC entry 4889 (class 2606 OID 22919)
-- Name: purchase_order purchase_order_warehouse_code_fk; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.purchase_order
    ADD CONSTRAINT purchase_order_warehouse_code_fk FOREIGN KEY (warehouse_code) REFERENCES public.warehouse(warehouse_code) ON DELETE SET NULL;


--
-- TOC entry 4895 (class 2606 OID 22924)
-- Name: purchase_request purchase_request_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.purchase_request
    ADD CONSTRAINT purchase_request_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- TOC entry 4900 (class 2606 OID 22929)
-- Name: purchase_request_line purchase_request_line_cost_subgroup_fk; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.purchase_request_line
    ADD CONSTRAINT purchase_request_line_cost_subgroup_fk FOREIGN KEY (cost_subgroup_id) REFERENCES public.cost_subgroup(id);


--
-- TOC entry 4901 (class 2606 OID 25930)
-- Name: purchase_request_line purchase_request_line_mat_code_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.purchase_request_line
    ADD CONSTRAINT purchase_request_line_mat_code_fkey FOREIGN KEY (mat_code) REFERENCES public.material_code(mat_code) DEFERRABLE;


--
-- TOC entry 4902 (class 2606 OID 23798)
-- Name: purchase_request_line purchase_request_line_memo_line_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.purchase_request_line
    ADD CONSTRAINT purchase_request_line_memo_line_id_fkey FOREIGN KEY (memo_line_id) REFERENCES public.memo_line(id);


--
-- TOC entry 4903 (class 2606 OID 22939)
-- Name: purchase_request_line purchase_request_line_pr_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.purchase_request_line
    ADD CONSTRAINT purchase_request_line_pr_id_fkey FOREIGN KEY (pr_id) REFERENCES public.purchase_request(id) ON DELETE CASCADE;


--
-- TOC entry 4896 (class 2606 OID 22944)
-- Name: purchase_request purchase_request_requested_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.purchase_request
    ADD CONSTRAINT purchase_request_requested_by_fkey FOREIGN KEY (requested_by) REFERENCES public.users(id);


--
-- TOC entry 4897 (class 2606 OID 22949)
-- Name: purchase_request purchase_request_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.purchase_request
    ADD CONSTRAINT purchase_request_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- TOC entry 4898 (class 2606 OID 22954)
-- Name: purchase_request purchase_request_warehouse_code_fk; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.purchase_request
    ADD CONSTRAINT purchase_request_warehouse_code_fk FOREIGN KEY (warehouse_code) REFERENCES public.warehouse(warehouse_code) ON DELETE SET NULL;


--
-- TOC entry 4899 (class 2606 OID 22959)
-- Name: purchase_request purchase_request_warehouse_code_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.purchase_request
    ADD CONSTRAINT purchase_request_warehouse_code_fkey FOREIGN KEY (warehouse_code) REFERENCES public.warehouse(warehouse_code);


--
-- TOC entry 4904 (class 2606 OID 22964)
-- Name: requisition requisition_checked_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.requisition
    ADD CONSTRAINT requisition_checked_by_fkey FOREIGN KEY (checked_by) REFERENCES public.users(id);


--
-- TOC entry 4907 (class 2606 OID 22969)
-- Name: requisition_line requisition_line_req_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.requisition_line
    ADD CONSTRAINT requisition_line_req_id_fkey FOREIGN KEY (req_id) REFERENCES public.requisition(id);


--
-- TOC entry 4908 (class 2606 OID 22974)
-- Name: requisition_line requisition_line_stock_item_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.requisition_line
    ADD CONSTRAINT requisition_line_stock_item_id_fkey FOREIGN KEY (stock_item_id) REFERENCES public.stock_item(id);


--
-- TOC entry 4905 (class 2606 OID 22979)
-- Name: requisition requisition_project_code_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.requisition
    ADD CONSTRAINT requisition_project_code_fkey FOREIGN KEY (project_code) REFERENCES public.project(project_code);


--
-- TOC entry 4906 (class 2606 OID 22984)
-- Name: requisition requisition_requester_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.requisition
    ADD CONSTRAINT requisition_requester_id_fkey FOREIGN KEY (requester_id) REFERENCES public.users(id);


--
-- TOC entry 4909 (class 2606 OID 22989)
-- Name: requisition_status_log requisition_status_log_changed_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.requisition_status_log
    ADD CONSTRAINT requisition_status_log_changed_by_fkey FOREIGN KEY (changed_by) REFERENCES public.users(id);


--
-- TOC entry 4910 (class 2606 OID 22994)
-- Name: requisition_status_log requisition_status_log_req_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.requisition_status_log
    ADD CONSTRAINT requisition_status_log_req_id_fkey FOREIGN KEY (req_id) REFERENCES public.requisition(id);


--
-- TOC entry 4912 (class 2606 OID 22999)
-- Name: rfq rfq_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.rfq
    ADD CONSTRAINT rfq_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- TOC entry 4915 (class 2606 OID 25935)
-- Name: rfq_line rfq_line_mat_code_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.rfq_line
    ADD CONSTRAINT rfq_line_mat_code_fkey FOREIGN KEY (mat_code) REFERENCES public.material_code(mat_code) DEFERRABLE;


--
-- TOC entry 4916 (class 2606 OID 23009)
-- Name: rfq_line rfq_line_rfq_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.rfq_line
    ADD CONSTRAINT rfq_line_rfq_id_fkey FOREIGN KEY (rfq_id) REFERENCES public.rfq(id) ON DELETE CASCADE;


--
-- TOC entry 4913 (class 2606 OID 23014)
-- Name: rfq rfq_pr_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.rfq
    ADD CONSTRAINT rfq_pr_id_fkey FOREIGN KEY (pr_id) REFERENCES public.purchase_request(id);


--
-- TOC entry 4914 (class 2606 OID 23019)
-- Name: rfq rfq_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.rfq
    ADD CONSTRAINT rfq_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- TOC entry 4917 (class 2606 OID 23024)
-- Name: role_menu_permissions role_menu_permissions_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.role_menu_permissions
    ADD CONSTRAINT role_menu_permissions_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- TOC entry 4918 (class 2606 OID 23029)
-- Name: role_menu_permissions role_menu_permissions_menu_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.role_menu_permissions
    ADD CONSTRAINT role_menu_permissions_menu_id_fkey FOREIGN KEY (menu_id) REFERENCES public.menus(id) ON DELETE CASCADE;


--
-- TOC entry 4919 (class 2606 OID 23034)
-- Name: role_menu_permissions role_menu_permissions_role_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.role_menu_permissions
    ADD CONSTRAINT role_menu_permissions_role_id_fkey FOREIGN KEY (role_id) REFERENCES public.roles(id) ON DELETE CASCADE;


--
-- TOC entry 4920 (class 2606 OID 23039)
-- Name: role_menu_permissions role_menu_permissions_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.role_menu_permissions
    ADD CONSTRAINT role_menu_permissions_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- TOC entry 4921 (class 2606 OID 23044)
-- Name: role_menus role_menus_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.role_menus
    ADD CONSTRAINT role_menus_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- TOC entry 4922 (class 2606 OID 23049)
-- Name: role_menus role_menus_menu_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.role_menus
    ADD CONSTRAINT role_menus_menu_id_fkey FOREIGN KEY (menu_id) REFERENCES public.menus(id) ON DELETE CASCADE;


--
-- TOC entry 4923 (class 2606 OID 23054)
-- Name: role_menus role_menus_role_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.role_menus
    ADD CONSTRAINT role_menus_role_id_fkey FOREIGN KEY (role_id) REFERENCES public.roles(id) ON DELETE CASCADE;


--
-- TOC entry 4924 (class 2606 OID 23059)
-- Name: role_permissions role_permissions_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.role_permissions
    ADD CONSTRAINT role_permissions_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- TOC entry 4925 (class 2606 OID 23064)
-- Name: role_permissions role_permissions_permission_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.role_permissions
    ADD CONSTRAINT role_permissions_permission_id_fkey FOREIGN KEY (permission_id) REFERENCES public.permissions(id) ON DELETE CASCADE;


--
-- TOC entry 4926 (class 2606 OID 23069)
-- Name: role_permissions role_permissions_role_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.role_permissions
    ADD CONSTRAINT role_permissions_role_id_fkey FOREIGN KEY (role_id) REFERENCES public.roles(id) ON DELETE CASCADE;


--
-- TOC entry 4928 (class 2606 OID 23074)
-- Name: roles roles_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.roles
    ADD CONSTRAINT roles_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- TOC entry 4929 (class 2606 OID 25701)
-- Name: roles roles_dept_code_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.roles
    ADD CONSTRAINT roles_dept_code_fkey FOREIGN KEY (dept_code) REFERENCES public.departments(dept_code) ON UPDATE CASCADE;


--
-- TOC entry 4930 (class 2606 OID 23084)
-- Name: roles roles_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.roles
    ADD CONSTRAINT roles_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- TOC entry 4931 (class 2606 OID 23089)
-- Name: spec_size spec_size_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.spec_size
    ADD CONSTRAINT spec_size_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- TOC entry 4932 (class 2606 OID 23094)
-- Name: spec_size spec_size_mat_name_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.spec_size
    ADD CONSTRAINT spec_size_mat_name_id_fkey FOREIGN KEY (mat_name_id) REFERENCES public.mat_name(id);


--
-- TOC entry 4933 (class 2606 OID 23099)
-- Name: spec_size spec_size_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.spec_size
    ADD CONSTRAINT spec_size_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- TOC entry 4934 (class 2606 OID 23104)
-- Name: stock_count stock_count_approved_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.stock_count
    ADD CONSTRAINT stock_count_approved_by_fkey FOREIGN KEY (approved_by) REFERENCES public.users(id);


--
-- TOC entry 4935 (class 2606 OID 23109)
-- Name: stock_count stock_count_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.stock_count
    ADD CONSTRAINT stock_count_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- TOC entry 4938 (class 2606 OID 23114)
-- Name: stock_count_line stock_count_line_count_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.stock_count_line
    ADD CONSTRAINT stock_count_line_count_id_fkey FOREIGN KEY (count_id) REFERENCES public.stock_count(id) ON DELETE CASCADE;


--
-- TOC entry 4939 (class 2606 OID 25940)
-- Name: stock_count_line stock_count_line_mat_code_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.stock_count_line
    ADD CONSTRAINT stock_count_line_mat_code_fkey FOREIGN KEY (mat_code) REFERENCES public.material_code(mat_code) DEFERRABLE;


--
-- TOC entry 4940 (class 2606 OID 23124)
-- Name: stock_count_line stock_count_line_zone_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.stock_count_line
    ADD CONSTRAINT stock_count_line_zone_id_fkey FOREIGN KEY (zone_id) REFERENCES public.storage_zone(id);


--
-- TOC entry 4936 (class 2606 OID 23129)
-- Name: stock_count stock_count_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.stock_count
    ADD CONSTRAINT stock_count_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- TOC entry 4937 (class 2606 OID 23134)
-- Name: stock_count stock_count_warehouse_code_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.stock_count
    ADD CONSTRAINT stock_count_warehouse_code_fkey FOREIGN KEY (warehouse_code) REFERENCES public.warehouse(warehouse_code);


--
-- TOC entry 4941 (class 2606 OID 23139)
-- Name: stock_inventory stock_inventory_item_fk; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.stock_inventory
    ADD CONSTRAINT stock_inventory_item_fk FOREIGN KEY (item_id) REFERENCES public.stock_item(id);


--
-- TOC entry 4942 (class 2606 OID 23144)
-- Name: stock_inventory stock_inventory_loc_fk; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.stock_inventory
    ADD CONSTRAINT stock_inventory_loc_fk FOREIGN KEY (location_code) REFERENCES public.location(location_code);


--
-- TOC entry 4943 (class 2606 OID 23149)
-- Name: stock_item stock_item_category_fk; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.stock_item
    ADD CONSTRAINT stock_item_category_fk FOREIGN KEY (category_id) REFERENCES public.stock_category(id) ON DELETE SET NULL;


--
-- TOC entry 4944 (class 2606 OID 23154)
-- Name: stock_item stock_item_created_by_fk; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.stock_item
    ADD CONSTRAINT stock_item_created_by_fk FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- TOC entry 4947 (class 2606 OID 23159)
-- Name: stock_item_image stock_item_image_item_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.stock_item_image
    ADD CONSTRAINT stock_item_image_item_id_fkey FOREIGN KEY (item_id) REFERENCES public.stock_item(id) ON DELETE CASCADE;


--
-- TOC entry 4945 (class 2606 OID 23164)
-- Name: stock_item stock_item_location_code_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.stock_item
    ADD CONSTRAINT stock_item_location_code_fkey FOREIGN KEY (location_code) REFERENCES public.location(location_code);


--
-- TOC entry 4946 (class 2606 OID 23169)
-- Name: stock_item stock_item_updated_by_fk; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.stock_item
    ADD CONSTRAINT stock_item_updated_by_fk FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- TOC entry 4948 (class 2606 OID 23174)
-- Name: stock_reservation stock_res_item_fk; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.stock_reservation
    ADD CONSTRAINT stock_res_item_fk FOREIGN KEY (item_id) REFERENCES public.stock_item(id);


--
-- TOC entry 4949 (class 2606 OID 23179)
-- Name: stock_reservation stock_res_loc_fk; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.stock_reservation
    ADD CONSTRAINT stock_res_loc_fk FOREIGN KEY (location_code) REFERENCES public.location(location_code);


--
-- TOC entry 4950 (class 2606 OID 23184)
-- Name: stock_reservation stock_res_user_fk; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.stock_reservation
    ADD CONSTRAINT stock_res_user_fk FOREIGN KEY (requested_by) REFERENCES public.users(id);


--
-- TOC entry 4951 (class 2606 OID 26167)
-- Name: stock_transaction stock_transaction_receive_document_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.stock_transaction
    ADD CONSTRAINT stock_transaction_receive_document_id_fkey FOREIGN KEY (receive_document_id) REFERENCES public.ic_po_receive_document(id);


--
-- TOC entry 4952 (class 2606 OID 26172)
-- Name: stock_transaction stock_transaction_return_document_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.stock_transaction
    ADD CONSTRAINT stock_transaction_return_document_id_fkey FOREIGN KEY (return_document_id) REFERENCES public.ic_po_return_document(id);


--
-- TOC entry 4955 (class 2606 OID 23808)
-- Name: stock_transfer stock_transfer_checked_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.stock_transfer
    ADD CONSTRAINT stock_transfer_checked_by_fkey FOREIGN KEY (checked_by) REFERENCES public.users(id);


--
-- TOC entry 4956 (class 2606 OID 23813)
-- Name: stock_transfer stock_transfer_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.stock_transfer
    ADD CONSTRAINT stock_transfer_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- TOC entry 4959 (class 2606 OID 23830)
-- Name: stock_transfer_line stock_transfer_line_item_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.stock_transfer_line
    ADD CONSTRAINT stock_transfer_line_item_id_fkey FOREIGN KEY (item_id) REFERENCES public.stock_item(id);


--
-- TOC entry 4960 (class 2606 OID 23189)
-- Name: stock_transfer_line stock_transfer_line_transfer_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.stock_transfer_line
    ADD CONSTRAINT stock_transfer_line_transfer_id_fkey FOREIGN KEY (transfer_id) REFERENCES public.stock_transfer(id);


--
-- TOC entry 4957 (class 2606 OID 23818)
-- Name: stock_transfer stock_transfer_requested_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.stock_transfer
    ADD CONSTRAINT stock_transfer_requested_by_fkey FOREIGN KEY (requested_by) REFERENCES public.users(id);


--
-- TOC entry 4961 (class 2606 OID 23835)
-- Name: stock_transfer_status_log stock_transfer_status_log_changed_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.stock_transfer_status_log
    ADD CONSTRAINT stock_transfer_status_log_changed_by_fkey FOREIGN KEY (changed_by) REFERENCES public.users(id);


--
-- TOC entry 4962 (class 2606 OID 23194)
-- Name: stock_transfer_status_log stock_transfer_status_log_transfer_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.stock_transfer_status_log
    ADD CONSTRAINT stock_transfer_status_log_transfer_id_fkey FOREIGN KEY (transfer_id) REFERENCES public.stock_transfer(id);


--
-- TOC entry 4958 (class 2606 OID 23823)
-- Name: stock_transfer stock_transfer_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.stock_transfer
    ADD CONSTRAINT stock_transfer_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- TOC entry 4953 (class 2606 OID 23199)
-- Name: stock_transaction stock_txn_created_fk; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.stock_transaction
    ADD CONSTRAINT stock_txn_created_fk FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- TOC entry 4954 (class 2606 OID 23204)
-- Name: stock_transaction stock_txn_item_fk; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.stock_transaction
    ADD CONSTRAINT stock_txn_item_fk FOREIGN KEY (item_id) REFERENCES public.stock_item(id);


--
-- TOC entry 4963 (class 2606 OID 23209)
-- Name: storage_zone storage_zone_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.storage_zone
    ADD CONSTRAINT storage_zone_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- TOC entry 4964 (class 2606 OID 23214)
-- Name: storage_zone storage_zone_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.storage_zone
    ADD CONSTRAINT storage_zone_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- TOC entry 4965 (class 2606 OID 23219)
-- Name: storage_zone storage_zone_warehouse_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.storage_zone
    ADD CONSTRAINT storage_zone_warehouse_id_fkey FOREIGN KEY (warehouse_id) REFERENCES public.warehouse(id);


--
-- TOC entry 4966 (class 2606 OID 23224)
-- Name: subgroup subgroup_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.subgroup
    ADD CONSTRAINT subgroup_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- TOC entry 4967 (class 2606 OID 23229)
-- Name: subgroup subgroup_group_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.subgroup
    ADD CONSTRAINT subgroup_group_id_fkey FOREIGN KEY (group_id) REFERENCES public.mat_group(id);


--
-- TOC entry 4968 (class 2606 OID 23234)
-- Name: subgroup subgroup_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.subgroup
    ADD CONSTRAINT subgroup_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- TOC entry 4969 (class 2606 OID 23239)
-- Name: supplier supplier_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.supplier
    ADD CONSTRAINT supplier_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- TOC entry 4970 (class 2606 OID 23244)
-- Name: supplier supplier_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.supplier
    ADD CONSTRAINT supplier_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- TOC entry 4971 (class 2606 OID 23249)
-- Name: unit unit_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.unit
    ADD CONSTRAINT unit_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- TOC entry 4972 (class 2606 OID 23254)
-- Name: unit unit_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.unit
    ADD CONSTRAINT unit_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- TOC entry 4973 (class 2606 OID 23259)
-- Name: user_menu_permissions user_menu_permissions_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.user_menu_permissions
    ADD CONSTRAINT user_menu_permissions_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- TOC entry 4974 (class 2606 OID 23264)
-- Name: user_menu_permissions user_menu_permissions_menu_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.user_menu_permissions
    ADD CONSTRAINT user_menu_permissions_menu_id_fkey FOREIGN KEY (menu_id) REFERENCES public.menus(id) ON DELETE CASCADE;


--
-- TOC entry 4975 (class 2606 OID 23269)
-- Name: user_menu_permissions user_menu_permissions_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.user_menu_permissions
    ADD CONSTRAINT user_menu_permissions_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- TOC entry 4976 (class 2606 OID 23274)
-- Name: user_menu_permissions user_menu_permissions_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.user_menu_permissions
    ADD CONSTRAINT user_menu_permissions_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- TOC entry 4977 (class 2606 OID 23279)
-- Name: user_permissions user_permissions_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.user_permissions
    ADD CONSTRAINT user_permissions_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- TOC entry 4978 (class 2606 OID 23284)
-- Name: user_permissions user_permissions_permission_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.user_permissions
    ADD CONSTRAINT user_permissions_permission_id_fkey FOREIGN KEY (permission_id) REFERENCES public.permissions(id) ON DELETE CASCADE;


--
-- TOC entry 4979 (class 2606 OID 23289)
-- Name: user_permissions user_permissions_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.user_permissions
    ADD CONSTRAINT user_permissions_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- TOC entry 4980 (class 2606 OID 23294)
-- Name: user_permissions user_permissions_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.user_permissions
    ADD CONSTRAINT user_permissions_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- TOC entry 4981 (class 2606 OID 23299)
-- Name: user_roles user_roles_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.user_roles
    ADD CONSTRAINT user_roles_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- TOC entry 4982 (class 2606 OID 23304)
-- Name: user_roles user_roles_role_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.user_roles
    ADD CONSTRAINT user_roles_role_id_fkey FOREIGN KEY (role_id) REFERENCES public.roles(id) ON DELETE CASCADE;


--
-- TOC entry 4983 (class 2606 OID 23309)
-- Name: user_roles user_roles_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.user_roles
    ADD CONSTRAINT user_roles_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- TOC entry 4985 (class 2606 OID 23314)
-- Name: users users_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- TOC entry 4986 (class 2606 OID 25706)
-- Name: users users_dept_code_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_dept_code_fkey FOREIGN KEY (dept_code) REFERENCES public.departments(dept_code) ON UPDATE CASCADE;


--
-- TOC entry 4987 (class 2606 OID 23324)
-- Name: users users_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- TOC entry 4988 (class 2606 OID 23329)
-- Name: warehouse warehouse_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.warehouse
    ADD CONSTRAINT warehouse_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- TOC entry 4989 (class 2606 OID 23334)
-- Name: warehouse warehouse_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.warehouse
    ADD CONSTRAINT warehouse_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- TOC entry 4991 (class 2606 OID 26448)
-- Name: work_order work_order_approver_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.work_order
    ADD CONSTRAINT work_order_approver_id_fkey FOREIGN KEY (approver_id) REFERENCES public.users(id);


--
-- TOC entry 4992 (class 2606 OID 23339)
-- Name: work_order_attachment work_order_attachment_wo_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.work_order_attachment
    ADD CONSTRAINT work_order_attachment_wo_id_fkey FOREIGN KEY (wo_id) REFERENCES public.work_order(id);


--
-- TOC entry 4993 (class 2606 OID 23344)
-- Name: work_order_cost_code work_order_cost_code_wo_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.work_order_cost_code
    ADD CONSTRAINT work_order_cost_code_wo_id_fkey FOREIGN KEY (wo_id) REFERENCES public.work_order(id);


--
-- TOC entry 5013 (class 2606 OID 26467)
-- Name: work_order_edit_log work_order_edit_log_edited_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.work_order_edit_log
    ADD CONSTRAINT work_order_edit_log_edited_by_fkey FOREIGN KEY (edited_by) REFERENCES public.users(id);


--
-- TOC entry 5014 (class 2606 OID 26462)
-- Name: work_order_edit_log work_order_edit_log_wo_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.work_order_edit_log
    ADD CONSTRAINT work_order_edit_log_wo_id_fkey FOREIGN KEY (wo_id) REFERENCES public.work_order(id) ON DELETE CASCADE;


--
-- TOC entry 4994 (class 2606 OID 23349)
-- Name: work_order_line work_order_line_wo_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.work_order_line
    ADD CONSTRAINT work_order_line_wo_id_fkey FOREIGN KEY (wo_id) REFERENCES public.work_order(id);


--
-- TOC entry 4995 (class 2606 OID 23354)
-- Name: work_order_payment_installment work_order_payment_installment_wo_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.work_order_payment_installment
    ADD CONSTRAINT work_order_payment_installment_wo_id_fkey FOREIGN KEY (wo_id) REFERENCES public.work_order(id);


--
-- TOC entry 4996 (class 2606 OID 23359)
-- Name: work_order_penalty work_order_penalty_wo_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.work_order_penalty
    ADD CONSTRAINT work_order_penalty_wo_id_fkey FOREIGN KEY (wo_id) REFERENCES public.work_order(id);


--
-- TOC entry 4997 (class 2606 OID 23364)
-- Name: work_order_retention work_order_retention_wo_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.work_order_retention
    ADD CONSTRAINT work_order_retention_wo_id_fkey FOREIGN KEY (wo_id) REFERENCES public.work_order(id);


--
-- TOC entry 4998 (class 2606 OID 23369)
-- Name: work_order_status_log work_order_status_log_wo_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.work_order_status_log
    ADD CONSTRAINT work_order_status_log_wo_id_fkey FOREIGN KEY (wo_id) REFERENCES public.work_order(id);


-- Completed on 2026-10-25 15:06:13

--
-- PostgreSQL database dump complete
--

\unrestrict OZc71CZbkAt997clByLeIPzKsV8VgtLPxMIfF6jgYtEAwxLgaiwd1mmJ8S5SmOk

